package vmm

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

var (
	CFPrefix = "matrisea-cvd-" // container name prefix
	// DefaultNetwork: although the ideal design is to setup a new bridge network so as
	// to ensure better isolation between matrisea and other docker workloads on the same host.
	// However, this could introduce in-container DNS failure on Ubuntu 18.09+ as 18.09+ defaults
	// to systemd-resolved for host DNS management. (More details https://github.com/moby/moby/issues/38243)
	DefaultNetwork   = "bridge"        // use docker's default bridge
	CFImage          = "cuttlefish"    // cuttlefish image name
	HomeDir          = "/home/vsoc-01" // workdir in container
	TimeoutVMStart   = 120 * time.Second
	HomeDirSizeLimit = 50 //soft disk quota for HomeDir
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// A `VM` is essentially a crosvm process running in a docker container.
// To avoid confusion between a `VM` and a `Container`, here by convention
//   - The word `VM` is used exclusively in exported functions for VM lifecycle management
//   - The word `container` is used else where for direct interaction with the underlying containers
//
// When setting up a new VM, the caller of should follow the call sequence below:
//   1. Create a folder in $DATA/devices/your-device-name and upload device images (system + CVD images)
//   2. VMCreate(your-device-name)
//   3. VMVMPreBootSetup() to install packages and start daemons
//   4. VMLoadFile() to copy the system image to the container's WorkDir
//   5. VMUnzipImage() to unzip the system image
//	 6. VMLoadFile() to copy CVD image to the container's WorkDir
//   7. VMStart()
type VMM struct {
	Client     *client.Client // Docker Engine client
	DataDir    string
	DevicesDir string
	DBDir      string
	UploadDir  string
	createMu   sync.Mutex // for concurrent CreateVM() call
}

type VMMError struct {
	msg string // description of error
}

type VMItem struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Created    string   `json:"created"` // unix timestamp
	Device     string   `json:"device"`
	IP         string   `json:"ip"`
	Status     VMStatus `json:"status"`
	Tags       []string `json:"tags"`
	CFInstance string   `json:"cf_instance"`
	CPU        int      `json:"cpu"`
	RAM        int      `json:"ram"`
	OSVersion  string   `json:"os_version"`
}

type VMStatus int

const (
	// container is up but crosvm not running
	VMReady VMStatus = iota
	// crosvm is running
	VMRunning VMStatus = iota
	// Container is in created/paused/restarting/removing/exited/dead status (not "running")
	// which shouldn't happen if the container is fully managed by Matrisea.
	// Require admin intervention to remove/resume using Docker CLI
	VMContainerError VMStatus = iota
)

// ExecResult represents a result returned from Exec()
type ExecResult struct {
	ExitCode  int
	outBuffer *bytes.Buffer
	errBuffer *bytes.Buffer
}

// Stdout returns stdout output of a command run by Exec()
func (res *ExecResult) Stdout() string {
	return res.outBuffer.String()
}

// Stderr returns stderr output of a command run by Exec()
func (res *ExecResult) Stderr() string {
	return res.errBuffer.String()
}

// Combined returns combined stdout and stderr output of a command run by Exec()
func (res *ExecResult) Combined() string {
	return res.outBuffer.String() + res.errBuffer.String()
}

func (e *VMMError) Error() string { return e.msg }

func NewVMM(dataDir string) (*VMM, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	// populate initial data folder structure
	devicesDir := path.Join(dataDir, "devices")
	dbDir := path.Join(dataDir, "db")
	uploadDir := path.Join(dataDir, "upload")

	folders := []string{
		dataDir,
		devicesDir,
		dbDir,
		uploadDir,
	}
	for _, f := range folders {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			err := os.Mkdir(f, 0755)
			if err != nil {
				return nil, err
			}
		}
	}

	log.Printf("DATA_DIR=%s\n", dataDir)

	v := &VMM{
		Client:     cli,
		DataDir:    dataDir,
		DevicesDir: devicesDir,
		DBDir:      dbDir,
		UploadDir:  uploadDir,
	}
	v.diskSheriff()

	return v, nil
}

// the caller is responsible for setting up device folder
// assume docker's default network exist on the host
func (v *VMM) VMCreate(deviceName string, cpu int, ram int, aospVersion string) (name string, err error) {
	ctx := context.Background()
	containerName := CFPrefix + deviceName

	// There will be a race condition on cfInstance if VMCreate() is called multiple times.
	// More specifically, findNextAvailableCFInstanceNumber() reads labels from existings containers.
	// If VMCreate() is called twice, both will get the same next available cf_instance as they both see the
	// same set of containers. By locking createMu, we ensure that one of the VMCreate() call
	// always complete first and finish creating a new container, so this new container will be counted towards the
	// next findNextAvailableCFInstanceNumber() call.
	v.createMu.Lock()
	defer v.createMu.Unlock()

	// The next available index of cuttlefish VM. Always >= 1.
	// It is important for us to keep tracking of this index as cuttlefish use it to derive different
	// vsock ports for each instance in launch_cvd
	cfInstance, err := v.getNextCFInstanceNumber()
	log.Printf("VMCreate: next available cf_instance %d", cfInstance)
	if err != nil {
		return "", err
	}
	websockifyPort, err := nat.NewPort("tcp", strconv.Itoa(6080+cfInstance-1))
	if err != nil {
		return "", err
	}
	// create a new VM
	containerConfig := &container.Config{
		Image:    CFImage,
		Hostname: containerName,
		Labels: map[string]string{ // for compatibility. Labels are used by android-cuttlefish CLI
			"cf_instance":               strconv.Itoa(cfInstance),
			"n_cf_instances":            "1",
			"vsock_guest_cid":           "true",
			"matrisea_device_name":      deviceName,
			"matrisea_cpu":              strconv.Itoa(cpu),
			"matrisea_ram":              strconv.Itoa(ram),
			"matrisea_aosp_version":     aospVersion,
			"matrisea_tag_aosp_version": aospVersion, // tags are for display only
		},
		Env: []string{
			"HOME=" + HomeDir,
		},
		ExposedPorts: nat.PortSet{
			websockifyPort: struct{}{},
		},
	}

	deviceDir := path.Join(v.DevicesDir, containerName)
	if _, err := os.Stat(deviceDir); os.IsNotExist(err) {
		return "", err
	}

	hostConfig := &container.HostConfig{
		Privileged: true,
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   "/sys/fs/cgroup",
				Target:   "/sys/fs/cgroup",
				ReadOnly: false,
			},
			{
				Type:     mount.TypeBind,
				Source:   deviceDir,
				Target:   "/data",
				ReadOnly: false,
			},
		},
		PortBindings: nat.PortMap{
			websockifyPort: []nat.PortBinding{
				{
					// expose websockify port
					HostIP:   "0.0.0.0",
					HostPort: strconv.Itoa(6080 + cfInstance - 1),
				},
			},
		},
	}

	// attach the container to the default bridge
	// the default bridge should have been created when the backend container started
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			DefaultNetwork: {},
		},
	}

	resp, err := v.Client.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", err
	}
	if err := v.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", err
	}

	log.Printf("Created VM %s %s cf_instance/%d\n", containerName, resp.ID, cfInstance)

	return containerName, nil
}

// Install necessary tools and start auxillary deamons in the VM's container
func (v *VMM) VMPreBootSetup(deviceName string) error {
	containerName := CFPrefix + deviceName

	err := v.installTools(containerName)
	if err != nil {
		return err
	}
	err = v.startVNCProxy(containerName)
	if err != nil {
		return err
	}
	return nil
}

// Run launch_cvd inside of a running container.
// Notice VMStart() doesn't guarentee succeesful VM boot. If launch_cvd takes more time than the timeout limit,
// launch_cvd will continue in the background and VMStart will return a timeout error.
//
// If isAsync is set to ture, we wait for the VM to boot, read stdout continuously, and return success only until we see
// VIRTUAL_DEVICE_BOOT_COMPLETED in the log. This mode is only used at VM creation time to ensure the new VM can
// boot successfuly for the first time.
// When isAysnc is true, the caller can supply a callback functions, which will be called to every time there's new console
// message from the launcher. The callback function can be used to stream live launch_cvd stdout/stderr.
func (v *VMM) VMStart(containerName string, isAsync bool, options string, callback func(string)) error {
	start := time.Now()
	cf_instance, err := v.getContainerCFInstanceNumber(containerName)
	if err != nil {
		return err
	}
	labels, err := v.getContainerLabels(containerName)
	if err != nil {
		return err
	}
	memory_gb, err := strconv.Atoi(labels["matrisea_ram"])
	if err != nil {
		return err
	}

	aospVersion, err := v.VMGetAOSPVersion(containerName)
	if err != nil {
		return err
	}
	// To show the files that define the flags, run `./bin/launch_cvd --help`
	//
	// vsock and network ports of cuttlefish containers are created in the host's namespace. To avoid conflict and
	// run multiple CVDs on the same host, we need to define both
	//    1. --vsock_guest_cid AND
	//    2. --base_instance_num (added in android_12_gsi for launch_cvd) OR CUTTLEFISH_INSTANCE (as env variable, works for android_gsi_{10-12})
	launch_cmd := []string{
		path.Join(HomeDir, "/bin/launch_cvd"),
		"--nostart_webrtc",
		"--start_vnc_server",
		fmt.Sprintf("--vsock_guest_cid=%d", cf_instance+2),
		fmt.Sprintf("--cpus=%s", labels["matrisea_cpu"]),
		fmt.Sprintf("--memory_mb=%d", memory_gb*1024),
		"--guest_audit_security=false",
		"--guest_enforce_security=false",
	}

	if aospVersion == "Android 12" {
		launch_cmd = append(launch_cmd, "--report_anonymous_usage_stats=y")
	}
	log.Println("VMStart cmdline: ", launch_cmd)

	// start cuttlefish device
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          launch_cmd,
		Tty:          true,
		Env:          []string{fmt.Sprintf("CUTTLEFISH_INSTANCE=%d", cf_instance)},
	})

	if err != nil {
		return err
	}

	// cmd only get executed after ContainerExecAttach
	aresp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return err
	}
	defer aresp.Close()

	// adb daemon needs to wait for the VM to boot in order to connect.
	// As we can't know for sure when the VM will start listening, our best chance to start adb daemon is to
	// wait for VMStart to complete/timeout
	defer func() {
		err = v.startADBDaemon(containerName)
		if err != nil {
			log.Println(containerName, err.Error())
		}
	}()

	if !isAsync {
		outputDone := make(chan int)

		go func() {
			scanner := bufio.NewScanner(aresp.Conn)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Println(line)
				callback(line)
				if strings.Contains(line, "VIRTUAL_DEVICE_BOOT_COMPLETED") {
					outputDone <- 1
				}
			}
			outputDone <- 0
		}()

		select {
		case done := <-outputDone:
			if done == 1 {
				elapsed := time.Since(start)
				log.Printf("VMStart successfully in %s\n", elapsed)
				return nil
			}
			return &VMMError{msg: "VMStart failed as launch_cvd terminated abnormally"}
		case <-time.After(TimeoutVMStart):
			return &VMMError{msg: "VMStart timeout"}
		}
	}
	return nil
}

// kill launch_cvd process in the container
func (v *VMM) VMStop(containerName string) error {
	fmt.Printf("StopVM: %s\n", containerName)
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{HomeDir + "/bin/stop_cvd"},
		Tty:          true,
	})
	if err != nil {
		return err
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return err
	}

	defer hijackedResp.Close()
	scanner := bufio.NewScanner(hijackedResp.Conn)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		if strings.Contains(line, "Successful") {
			return nil
		}
	}
	return &VMMError{msg: "failed to stop the VM"}
}

func (v *VMM) VMLoadFile(containerName string, srcPath string) error {
	return v.containerCopyFile(srcPath, containerName, HomeDir)
}

func (v *VMM) VMUnzipImage(containerName string, imageFile string) error {
	match, _ := regexp.MatchString("^[a-zA-z0-9-_]+\\.zip$", imageFile)
	if !match {
		return &VMMError{"Failed to unzip due to invalid zip filename \"" + imageFile + "\""}
	}
	log.Printf("Unzip %s in container %s at %s", imageFile, containerName, HomeDir)
	_, err := v.containerExec(containerName, "unzip "+path.Join(HomeDir, imageFile), "vsoc-01")
	return err
}

func (v *VMM) VMRemove(containerName string) error {
	containerID, err := v.getContainerIDByName(containerName)
	if err != nil {
		return err
	}

	err = v.Client.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{
		Force: true,
		// required since /home/vsoc-01 is mounted as an anonymous volume
		RemoveVolumes: true,
	})
	if err != nil {
		return err
	}
	err = os.RemoveAll(path.Join(v.DevicesDir, containerName))
	if err != nil {
		return err
	}
	return nil
}

// remove all managed VMs
func (v *VMM) VMPrune() {
	log.Println("PruneVMs called")
	cfList, _ := v.listCuttlefishContainers()
	for _, c := range cfList {
		err := v.VMRemove(c.Names[0][1:])
		if err != nil {
			panic(err)
		}
		log.Printf("Removed VM %s\n", c.ID[:10])
	}
}

func (v *VMM) VMList() ([]VMItem, error) {
	cfList, err := v.listCuttlefishContainers()
	if err != nil {
		return nil, err
	}

	resp := []VMItem{}
	for _, c := range cfList {
		containerName := c.Names[0][1:]
		status, err := v.getVMStatus(containerName)
		if err != nil {
			return nil, err
		}
		cpu, err := strconv.Atoi(c.Labels["matrisea_cpu"])
		if err != nil {
			cpu = 0
		}
		ram, err := strconv.Atoi(c.Labels["matrisea_ram"])
		if err != nil {
			ram = 0
		}
		tags := []string{}
		for key, element := range c.Labels {
			if strings.HasPrefix(key, "matrisea_tag_") {
				tags = append(tags, element)
			}
		}

		resp = append(resp, VMItem{
			ID:         c.ID,
			Name:       c.Labels["matrisea_device_name"],
			Created:    strconv.FormatInt(c.Created, 10),
			Device:     c.Labels["matrisea_device_template"],
			IP:         c.NetworkSettings.Networks[DefaultNetwork].IPAddress,
			Status:     status,
			CFInstance: c.Labels["cf_instance"],
			OSVersion:  c.Labels["matrisea_aosp_version"],
			CPU:        cpu,
			RAM:        ram,
			Tags:       tags,
		})
	}
	return resp, nil
}

func (v *VMM) VMGetAOSPVersion(containerName string) (string, error) {
	containerJSON, err := v.getContainerJSON(containerName)
	if err != nil {
		return "", err
	}
	return containerJSON.Config.Labels["matrisea_aosp_version"], nil
}

func (v *VMM) VMInstallAPK(containerName string, apkFile string) error {
	f := path.Join(v.DevicesDir, containerName, apkFile)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		log.Printf("Abort installAPK because %s does not exist", f)
		return &VMMError{"Apk file does not exist"}
	}
	// adb daemon may have been terminated at this point so let's bring it up
	err := v.startADBDaemon(containerName)
	if err != nil {
		return err
	}
	resp, err := v.containerExec(containerName, "adb install \"/data/"+apkFile+"\"", "vsoc-01")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"non-zero exit in installAPK: " + resp.errBuffer.String()}
	}
	return nil
}

// Start a bash shell in the container and returns a bi-directional stream for the frontend to interact with.
// It's up to the caller to close the hijacked connection by calling types.HijackedResponse.Close.
// It's up to the caller to call KillTerminal() to kill the long running process at exit
func (v *VMM) ContainerAttachToTerminal(containerName string) (hr types.HijackedResponse, err error) {
	log.Printf("ExecAttachToTerminal %s\n", containerName)
	// TODO to do it properly, might need to get terminal dimensions from the front end
	// and dynamically adjust docker's tty dimensions
	// reference: https://github.com/xtermjs/xterm.js/issues/1359
	cmd := []string{"/bin/bash"}
	env := []string{"COLUMNS=205", "LINES=40"}
	return v.ContainerAttachToProcess(containerName, cmd, env)
}

// Start a long running process with TTY and returns a bi-directional stream for the frontend to interact with.
// It's up to the caller to close the hijacked connection by calling types.HijackedResponse.Close.
// It's up to the caller to call KillTerminal() to kill the long running process at exit. (see reason below)
//
// Explanation: types.HijackedResponse.Close only calls HijackedResponse.Conn.Close() which leaves the process in the
// container to run forever. Moby's implementation of ContainerExecStart only terminates the process when either
// the context is Done or the attached stream returns EOF/error. In our use cases (e.g. bash/tail -f), the only possible
// way to terminate such long running processes by API is through context. However, if we trace ContainerExecAttach,
// Eventually we will end up at...
//
// # api/server/router/container/exec.go#L132
// // Now run the user process in container.
// // Maybe we should we pass ctx here if we're not detaching?
// s.backend.ContainerExecStart(context.Background(), ...)
//
// https://github.com/moby/moby/blob/7b9275c0da707b030e62c96b679a976f31f929d3/api/server/router/container/exec.go#L132
//
// ... which always create a new context.Background(). Apparantly Moby team didn't implement the `maybe` part that allows
// context passing.
func (v *VMM) ContainerAttachToProcess(containerName string, cmd []string, env []string) (hr types.HijackedResponse, err error) {
	ctx := context.Background()
	ir, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
		Tty:          true,
		Env:          env,
	})
	if err != nil {
		return types.HijackedResponse{}, err
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, ir.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return hijackedResp, err
	}
	return hijackedResp, nil
}

// Kill the bash process after use. To be called after done with the process created by ExecAttachToTerminal().
func (v *VMM) ContainerKillTerminal(containerName string) error {
	return v.ContainerKillProcess(containerName, "/bin/bash")
}

// Kill all process in the given container with the given cmd. To be called after done with the process created by ExecAttachToTTYProcess().
//
// This is an ugly workaround since Moby's exec kill is long overdue (since 2014 https://github.com/moby/moby/pull/41548)
// Unfortunately we have to kill all pids of the same cmd since we can't get the specific terminal's pid in the container's
// pid namespace. This is because when creating a terminal in AttachToTerminal(), ContainerExecCreate only returns
// an execID that links to the spawned process's pid in the HOST pid namespace. We can't directly kill a host process unless
// we run the API server as root, which is undesirable.
func (v *VMM) ContainerKillProcess(containerName string, cmd string) error {
	process := strings.Split(cmd, " ")[0]
	resp, err := v.containerExec(containerName, fmt.Sprintf("ps -ef | awk '$8==\"%s\" {print $2}'", process), "vsoc-01")
	if err != nil {
		return err
	}
	pids := strings.Split(resp.outBuffer.String(), "\n")
	if len(pids) == 0 {
		log.Printf("Failed to kill process %s in container %s due to no matched pid found\n", process, containerName)
	}
	for _, pid := range pids {
		if pid != "" {
			_, err := v.containerExec(containerName, fmt.Sprintf("kill %s", pid), "root")
			if err != nil {
				// kill with best effort so just do logging
				log.Printf("Failed to kill process %s in container %s due to %s\n", pid, containerName, err.Error())
				continue
			}
			log.Printf("Killed process (%s)%s in container %s", pid, process, containerName)
		}
	}
	return nil
}

// Get a list of files in the given container's path, equivalent to running find {folder} -maxdepth 1 -printf "%M|%u|%g|%s|%A@|%P\n"
// Results are of the following format which each line represents a file/folder
// -rw-r--r--|vsoc-01|vsoc-01|65536|1645183964.5579601750|vbmeta.img
func (v *VMM) ContainerListFiles(containerName string, folder string) ([]string, error) {
	folder = path.Clean(folder)
	resp, err := v.containerExec(containerName, "find "+folder+" -maxdepth 1 -printf \"%M|%u|%g|%s|%A@|%P\n\" | sort -t '|' -k6", "vsoc-01")
	if err != nil || resp.ExitCode != 0 {
		return []string{}, err
	}
	lines := strings.Split(resp.outBuffer.String(), "\n")
	// remove the last empty line due to split
	return lines[:len(lines)-1], nil
}

// Get content of a file in the container, in the form of bytes
// Due to CopyFromContainer()'s limitation we can only get a single file as tar archive so we have to untar it in memory
func (v *VMM) ContainerReadFile(containerName string, filePath string) ([]byte, error) {
	id, err := v.getContainerIDByName(containerName)
	if err != nil {
		return []byte{}, err
	}
	log.Printf("Copying file %s in container %s", filePath, containerName)
	reader, _, err := v.Client.CopyFromContainer(context.TODO(), id, filePath)
	if err != nil {
		log.Println(err.Error())
	}
	tr := tar.NewReader(reader)

	// first param is the header of the tar file
	_, err = tr.Next()
	if err == io.EOF {
		// end of tar archive
		log.Printf("Failed to read file %s in container %s because tar is empty", filePath, containerName)
		return []byte{}, err
	}
	if err != nil {
		log.Printf("Failed to read file %s in container %s because %s", filePath, containerName, err.Error())
		return []byte{}, err
	}
	buf := new(bytes.Buffer)
	// TODO don't untar large files. Return tar directly
	buf.ReadFrom(tr)
	log.Printf("Read file %s in container %s, size %d", filePath, containerName, buf.Len())
	return buf.Bytes(), nil
}

// Get the next smallest cf_instance number that have not been assigned
func (v *VMM) getNextCFInstanceNumber() (int, error) {
	cfList, err := v.listCuttlefishContainers()
	if err != nil {
		return -1, err
	}
	indexes := []int{}
	for _, c := range cfList {
		cf_idx, err := strconv.Atoi(c.Labels["cf_instance"])
		if err != nil {
			return -1, err
		}
		indexes = append(indexes, cf_idx)
	}
	sort.Ints(indexes)
	// if all cf_instance numbers so far are continuous
	if indexes[len(indexes)-1] == len(cfList) {
		return len(cfList) + 1, nil
	} else {
		i := 1
		for {
			if indexes[i-1] != i {
				return i, nil
			}
			i = i + 1
		}
	}
}

func (v *VMM) getContainerCFInstanceNumber(containerName string) (int, error) {
	containerJSON, err := v.getContainerJSON(containerName)
	if err != nil {
		return -1, err
	}
	num, err := strconv.Atoi(containerJSON.Config.Labels["cf_instance"])
	if err != nil {
		return -1, err
	}
	return num, nil
}

func (v *VMM) getContainerIP(containerName string) (string, error) {
	containerJSON, err := v.getContainerJSON(containerName)
	if err != nil {
		return "", err
	}
	return containerJSON.NetworkSettings.IPAddress, nil
}

func (v *VMM) getContainerLabels(containerName string) (map[string]string, error) {
	containerJSON, err := v.getContainerJSON(containerName)
	if err != nil {
		return nil, err
	}
	return containerJSON.Config.Labels, nil
}

func (v *VMM) getContainerJSON(containerName string) (types.ContainerJSON, error) {
	cid, err := v.getContainerIDByName(containerName)
	if err != nil {
		return types.ContainerJSON{}, err
	}
	return v.Client.ContainerInspect(context.Background(), cid)
}

// Install websockify and listen to websocket-based VNC connection on the container port 6080
//
// When a cuttlefish VM is created with --start-vnc-server flag, /home/vsoc-01/bin/vnc_server starts to listen
// on 6444 of the `lo` interface. This vnc_server only supports RFB 3.x which isn't compatible with the websocket-based
// protocol of novnc.js. To allow the frontend to access the VNC stream inside of the container, we need to both
// translate RFB to websocket and to listen to a port on the container's `eth0` interface. websockify can do both.
//
// Notice that websockify only listen on eth0 inside of the container which isn't reachable from outside of the host.
// The caller of this function is responsible to setup a reverse proxy to enable external VNC access.
func (v *VMM) startVNCProxy(containerName string) error {
	cfIndex, err := v.getContainerCFInstanceNumber(containerName)
	if err != nil {
		return &VMMError{"Failed to get VMInstanceNumber"}
	}
	vncPort := 6444 + cfIndex - 1
	wsPort := 6080 + cfIndex - 1
	resp, err := v.containerExec(containerName, fmt.Sprintf("websockify -D %d 127.0.0.1:%d --log-file websockify.log", wsPort, vncPort), "vsoc-01")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to start websockify, reason:" + resp.errBuffer.String()}
	}
	log.Println("websockify daemon started")
	return nil
}

// Start an adb daemon in the container and connect to the VM
// The function should be called when VM has booted up and started listening on the adb port
// The function is safe to be called repeatedly as adb will ignore duplicated connect commands and return "already connected"
func (v *VMM) startADBDaemon(containerName string) error {
	cfIndex, err := v.getContainerCFInstanceNumber(containerName)
	if err != nil {
		return &VMMError{"Failed to get VMInstanceNumber"}
	}
	adbPort := 6520 + cfIndex - 1
	ip, err := v.getContainerIP(containerName)
	if err != nil {
		return &VMMError{"Failed to get container IP"}
	}
	resp, err := v.containerExec(containerName, fmt.Sprintf("adb connect %s:%d", ip, adbPort), "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to start adb daemon, reason" + resp.errBuffer.String()}
	}
	log.Printf("adb daemon connected to %s:%d", ip, adbPort)
	log.Print("adb stdout:" + resp.outBuffer.String())
	log.Print("adb stderr:" + resp.outBuffer.String())
	return nil
}

func (v *VMM) installTools(containerName string) error {
	resp, err := v.containerExec(containerName, "apt install -y -qq adb git htop python3-pip iputils-ping less websockify", "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to apt install additional tools, reason:" + resp.errBuffer.String()}
	}
	resp, err = v.containerExec(containerName, "pip3 install frida-tools", "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to install python packages, reason:" + resp.errBuffer.String()}
	}
	return nil
}

func (v *VMM) getContainerIDByName(target string) (containerID string, err error) {
	cfList, err := v.listCuttlefishContainers()
	if err != nil {
		return "", err
	}
	for _, c := range cfList {
		for _, name := range c.Names {
			// docker container names all start with "/"
			prefix := "/" + CFPrefix
			if strings.HasPrefix(name, prefix) && strings.Contains(name, target) {
				return c.ID, nil
			}
		}
	}
	return "", nil
}

// copy a single file into the container
// if srcPath isn't a tar, it will be tar-ed in a temporary folder first
func (v *VMM) containerCopyFile(srcPath string, containerName string, dstPath string) error {
	log.Printf("Copy file into container %s:\n", containerName)
	log.Printf("  src: %s\n", srcPath)
	log.Printf("  dst: %s\n", dstPath)
	start := time.Now()

	if strings.HasSuffix(srcPath, ".tar") {
		v.containerCopyTarFile(srcPath, containerName, dstPath)
	}

	tmpdir, err := ioutil.TempDir("", "matrisea")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)
	srcFolder, srcFile := filepath.Split(srcPath)

	cmdStr := fmt.Sprintf("cd %s && tar -cvzf %s/%s.tar %s", srcFolder, tmpdir, srcFile, srcFile)

	// TODO read stderr and always print to console
	cmd := exec.Command("sh", "-c", cmdStr)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Println(fmt.Sprint(err) + ": " + stderr.String())
		return err
	}

	archive := tmpdir + "/" + srcFile + ".tar"
	if err = v.containerCopyTarFile(archive, containerName, dstPath); err != nil {
		return err
	}

	elapsed := time.Since(start)
	log.Printf("  Copy completed in %s\n", elapsed)
	return nil
}

// wrapper function on docker's CopyToContainer API where the srcPath must be a tar file
// The API will fail silently if srcPath isn't a tar
func (v *VMM) containerCopyTarFile(srcPath string, containerName string, dstPath string) error {
	containerID, err := v.getContainerIDByName(containerName)
	if err != nil {
		return err
	}

	archive, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer archive.Close()

	err = v.Client.CopyToContainer(context.Background(), containerID, dstPath, bufio.NewReader(archive), types.CopyToContainerOptions{})
	if err != nil {
		return err
	}
	return nil
}

// Execute a command in a container and return the result
// containing stdout, stderr, and exit code. Note:
//  - this is a synchronous operation;
//  - cmd stdin is closed.
//
// Adapted from moby's exec implementation
// source: https://github.com/moby/moby/blob/master/integration/internal/container/exec.go
func (v *VMM) containerExec(containerName string, cmd string, user string) (ExecResult, error) {
	// start := time.Now()
	ctx := context.Background()
	// prepare exec
	execConfig := types.ExecConfig{
		User:         user,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-c", cmd},
	}
	cresp, err := v.Client.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return ExecResult{}, err
	}
	execID := cresp.ID

	// run it, with stdout/stderr attached
	aresp, err := v.Client.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return ExecResult{}, err
	}
	defer aresp.Close()

	// read the output
	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error, 1)

	go func() {
		// StdCopy demultiplexes the stream into two buffers
		_, err = stdcopy.StdCopy(&outBuf, &errBuf, aresp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return ExecResult{}, err
		}
		break

	case <-ctx.Done():
		return ExecResult{}, ctx.Err()
	}

	// get the exit code
	iresp, err := v.Client.ContainerExecInspect(ctx, execID)
	if err != nil {
		return ExecResult{}, err
	}

	// elapsed := time.Since(start)
	// if iresp.ExitCode != 0 {
	// 	log.Printf("ContainerExec %s: %s\n", containerName, cmd)
	// 	log.Printf("  ExitCode: %d\n", iresp.ExitCode)
	// 	log.Printf("  stdout: %s\n", outBuf.String())
	// 	log.Printf("  stderr: %s\n", errBuf.String())
	// 	log.Printf("  ContainerExec completed in %s\n", elapsed)
	// }
	return ExecResult{ExitCode: iresp.ExitCode, outBuffer: &outBuf, errBuffer: &errBuf}, nil
}

func (v *VMM) listCuttlefishContainers() ([]types.Container, error) {
	ctx := context.Background()
	containers, err := v.Client.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	cflist := []types.Container{}
	for _, c := range containers {
		if v.isCuttlefishContainer(c) {
			cflist = append(cflist, c)
		}
	}
	return cflist, nil
}

func (v *VMM) getVMStatus(containerName string) (VMStatus, error) {
	containerJSON, err := v.getContainerJSON(containerName)
	if err != nil {
		return -1, err
	}
	// String representation of the container state
	// Can be one of "created", "running", "paused", "restarting", "removing", "exited", or "dead"
	if containerJSON.State.Status == "running" {
		// use grep "[x]xxx" technique to prevent grep itself from showing up in the ps result
		resp, err := v.containerExec(containerName, "ps aux|grep \"[l]aunch_cvd\"", "vsoc-01")
		if err != nil {
			return -1, err
		}
		if strings.Contains(resp.outBuffer.String(), "launch_cvd") {
			return VMRunning, nil
		}
		return VMReady, nil
	}
	// log.Printf("Unexpected status %s of container %s\n", containerJSON.State.Status, containerName)
	return VMContainerError, nil
}

// get the VMPrefix name of a cuttlefish container
// if it is not a cuttlefish container, return ""
func (v *VMM) isCuttlefishContainer(container types.Container) bool {
	for _, name := range container.Names {
		// docker container names all start with "/"
		prefix := "/" + CFPrefix
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Periodically Check container volume usage (/home/vsoc-01) and stop the VM if the volume size has exceeded the limit
//
// This is because cuttlefish forces restart on all crashed subprocesses and there is no option to override such behavior.
// If a VM has crashed and entered an unrecovable state, launch_cvd will enter a boot loop, generates large amount of launcher log,
// and eventually fill up the entire disk. Docker's disk quota feature (via --storage-opt) won't help in this case as
// the feature relies docker's overlayfs2 driver to use a non-default xfs backing fs.
// To prevent this rare yet devastating scenario a.k.a. device entering a boot loop and left running forever, diskShriff() runs
// periodically to check if the container's /home/vsoc-01 volume has exceeded a given limit. If true, VMStop is called against the VM.
func (v *VMM) diskSheriff() {
	go func() {
		for {
			fmt.Println("Run DiskSheriff")
			containers, err := v.listCuttlefishContainers()
			if err != nil {
				log.Printf("DiskSheriff failed to list containers due to %\n", err.Error())
			}

			for _, c := range containers {
				containerName := c.Names[0][1:]
				status, err := v.getVMStatus(containerName)
				if err != nil {
					log.Printf("DiskSheriff failed to get VMStatus due to %\n", err.Error())
				}
				if status == VMRunning {
					volSize, err := v.getContainerVolumeUsage(containerName)
					if err != nil {
						log.Printf("DiskSheriff failed to get volume usage due to %\n", err.Error())
					}
					// fmt.Printf("DiskSheriff,%s,%f\n", containerName, float64(volSize)/(math.Pow(1024, 3)))
					// TODO read limit from container labels
					if float64(volSize)/(math.Pow(1024, 3)) > float64(HomeDirSizeLimit) {
						log.Printf("DiskSheriff: VM %s has exceeded disk limit, probably in a boot loop, stopping now\n", containerName)
						if err := v.VMStop(containerName); err != nil {
							log.Printf("DiskSheriff: failed to stop VM %s due to %s\n", containerName)
						}
					}
				}
			}
			time.Sleep(30 * time.Second)
		}
	}()
}

func (v *VMM) getContainerVolumeUsage(containerName string) (int64, error) {
	// Volume.UsageData.Size is only populates by DiskUsage()
	du, err := v.Client.DiskUsage(context.Background())
	if err != nil {
		log.Printf("getContainerVolumeUsage failed due to %\n", err.Error())
	}
	c, err := v.getContainerJSON(containerName)
	if err != nil {
		log.Printf("getContainerVolumeUsage failed due to %\n", err.Error())
	}
	for _, m := range c.Mounts {
		if m.Destination == HomeDir {
			for _, vol := range du.Volumes {
				if m.Name == vol.Name {
					return vol.UsageData.Size, nil
				}
			}
		}
	}
	return 0, &VMMError{fmt.Sprintf("Couldn't find %s volume in container %s", HomeDir, containerName)}
}

func init() {
	rand.Seed(time.Now().UnixNano())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
