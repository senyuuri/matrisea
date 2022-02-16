package vmm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// for DefaultNetwork: although the ideal design is to setup a new bridge network so as
// to ensure better isolation between matrisea and other docker workloads on the same host.
// However, this could introduce in-container DNS failure on Ubuntu 18.09+ as 18.09+ defaults
// to systemd-resolved for host DNS management.
//
// More details: https://github.com/moby/moby/issues/38243
var (
	CFPrefix       = "matrisea-cvd-" // container name prefix
	DefaultNetwork = "bridge"        // use docker's default bridge
	CFImage        = "cuttlefish"    // cuttlefish image name
	HomeDir        = "/home/vsoc-01" // workdir in container
	TimeoutVMStart = 120 * time.Second
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// In Matrisea, a cuttlefish VM is essentially a crosvm process running in a docker container.
// To avoid confusion between a `VM` and a `Container`, here by convention
//   - The word `VM` is used exclusively in exported functions for VM lifecycle management
//   - The word `container` is used else where for direct interaction with the underlying containers
//
// Caller of VMM is responsible to follow the call sequence when setting up a VM:
//   1. Create a folder in $DATA/devices/your-device-name and upload device images
//   2. VMCreate(your-device-name)
//   3. VMLoadFile() to copy images to the container's WorkDir
//   4. VMStart()
type VMM struct {
	Client     *client.Client // Docker Engine client
	DataDir    string
	DevicesDir string
	DBDir      string
	UploadDir  string
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
}

type VMs []VMItem

type VMStatus int

const (
	VMReady   VMStatus = iota // container is up but crosvm not running
	VMRunning VMStatus = iota // crosvm is running
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

	return &VMM{
		Client:     cli,
		DataDir:    dataDir,
		DevicesDir: devicesDir,
		DBDir:      dbDir,
		UploadDir:  uploadDir,
	}, nil
}

// the caller is responsible for setting up device folder
// assume docker's default network exist on the host
func (v *VMM) VMCreate(deviceName string, cpu int, ram int, aospVersion string) (name string, err error) {
	ctx := context.Background()
	containerName := CFPrefix + deviceName

	// The next available index of cuttlefish VM. Always >= 1.
	// It is important for us to keep tracking of this index as cuttlefish use it to derive different
	// vsock ports for each instance in launch_cvd
	vmCount, err := v.countVM()
	if err != nil {
		return "", err
	}
	cfIndex := vmCount + 1
	websockifyPort, err := nat.NewPort("tcp", strconv.Itoa(6080+cfIndex-1))
	if err != nil {
		return "", err
	}
	// create a new VM
	containerConfig := &container.Config{
		Image:    CFImage,
		Hostname: containerName,
		Labels: map[string]string{ // for compatibility. Labels are used by android-cuttlefish CLI
			"cf_instance":               strconv.Itoa(cfIndex),
			"n_cf_instances":            "1",
			"vsock_guest_cid":           "true",
			"matrisea_device_name":      deviceName,
			"matrisea_cpu":              strconv.Itoa(cpu),
			"matrisea_ram":              strconv.Itoa(ram),
			"matrisea_tag_aosp_version": aospVersion,
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
					HostPort: strconv.Itoa(6080 + cfIndex - 1),
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
	// start auxillary deamons
	err = v.installTools(containerName)
	if err != nil {
		return "", err
	}
	err = v.startVNCProxy(containerName)
	if err != nil {
		return "", err
	}

	log.Printf("Created VM %s %s cf_instance/%d\n", containerName, resp.ID, cfIndex)

	return containerName, nil
}

// run launch_cvd inside of a running container
// notice VMStart() doesn't guarentee succeesful VM boot if the boot process takes more than timeout
func (v *VMM) VMStart(containerName string, isAsync bool, options string) (err error) {
	start := time.Now()
	cf_instance, err := v.GetVMInstanceNum(containerName)
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
	// To show the files that define the flags, run bin/launch_cvd --help
	//
	// TODO add support for cvd-host_packages in Android 11 and below
	// TODO ask user to choose android version when creating CVDs
	//
	// Both --vsock_guest_cid and --base_instance_num are required to allow running multiple CVDs in the same host.
	// However, such flags are only included in the cvd-host_packages.tar since aosp_12_gsi. Supplying the flags
	// to an image of Android 11 or below will result in error and cause launch_cvd to abort at the start.
	launch_cmd := []string{
		path.Join(HomeDir, "/bin/launch_cvd"),
		"--nostart_webrtc",
		"--start_vnc_server",
		fmt.Sprintf("--base_instance_num=%d", cf_instance), //added in aosp 12 gsi
		fmt.Sprintf("--vsock_guest_cid=%d", cf_instance+2), //added in aosp 12 gsi
		"--report_anonymous_usage_stats=y",                 //added in aosp 12 gsi
		fmt.Sprintf("--cpus=%s", labels["matrisea_cpu"]),
		fmt.Sprintf("--memory_mb=%d", memory_gb*1024),
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

	// If isAsync is ture, we wait for the VM to boot, read stdout continuously, and return success only until we see
	// VIRTUAL_DEVICE_BOOT_COMPLETED in the log. This mode is only used at VM creation time to ensure the new VM can
	// boot successfuly for the first time.
	if !isAsync {
		outputDone := make(chan int)

		go func() {
			scanner := bufio.NewScanner(aresp.Conn)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Println(line)
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
			return &VMMError{msg: "VMStart EOF while reading output"}
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
	_, err := v.ContainerExec(containerName, "unzip "+path.Join(HomeDir, imageFile), "vsoc-01")
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

func (v *VMM) VMList() (VMs, error) {
	cfList, err := v.listCuttlefishContainers()
	if err != nil {
		return nil, err
	}

	resp := VMs{}
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
			CPU:        cpu,
			RAM:        ram,
			Tags:       tags,
		})
	}
	return resp, nil
}

func (v *VMM) countVM() (int, error) {
	cfList, err := v.listCuttlefishContainers()
	if err != nil {
		return -1, err
	}
	return len(cfList), nil
}

func (v *VMM) GetVMInstanceNum(containerName string) (int, error) {
	cid, err := v.getContainerIDByName(containerName)
	if err != nil {
		return -1, err
	}
	containerJSON, err := v.Client.ContainerInspect(context.Background(), cid)
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
	cid, err := v.getContainerIDByName(containerName)
	if err != nil {
		return "", err
	}
	containerJSON, err := v.Client.ContainerInspect(context.Background(), cid)
	if err != nil {
		return "", err
	}
	return containerJSON.NetworkSettings.IPAddress, nil
}

func (v *VMM) getContainerLabels(containerName string) (map[string]string, error) {
	cid, err := v.getContainerIDByName(containerName)
	if err != nil {
		return nil, err
	}
	containerJSON, err := v.Client.ContainerInspect(context.Background(), cid)
	if err != nil {
		return nil, err
	}
	return containerJSON.Config.Labels, nil
}

// Start a bash shell in the container and returns a bi-directional stream for the frontend to interact with.
// It's up to the caller to close the hijacked connection by calling types.HijackedResponse.Close.
// It's up to the caller to call KillTerminal() to kill the long running process at exit
func (v *VMM) ExecAttachToTerminal(containerName string) (hr types.HijackedResponse, err error) {
	log.Printf("ExecAttachToTerminal %s\n", containerName)
	// TODO to do it properly, might need to get terminal dimensions from the front end
	// and dynamically adjust docker's tty dimensions
	// reference: https://github.com/xtermjs/xterm.js/issues/1359
	cmd := []string{"/bin/bash"}
	env := []string{"COLUMNS=205", "LINES=40"}
	return v.ExecAttachToTTYProcess(containerName, cmd, env)
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
func (v *VMM) ExecAttachToTTYProcess(containerName string, cmd []string, env []string) (hr types.HijackedResponse, err error) {
	ctx := context.Background()
	ir, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
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
func (v *VMM) KillTerminal(containerName string) error {
	return v.KillTTYProcess(containerName, "/bin/bash")
}

// Kill all process in the given container with the given cmd. To be called after done with the process created by ExecAttachToTTYProcess().
//
// This is an ugly workaround since Moby's exec kill is long overdue (since 2014 https://github.com/moby/moby/pull/41548)
// Unfortunately we have to kill all pids of the same cmd since we can't get the specific terminal's pid in the container's
// pid namespace. This is because when creating a terminal in AttachToTerminal(), ContainerExecCreate only returns
// an execID that links to the spawned process's pid in the HOST pid namespace. We can't directly kill a host process unless
// we run the API server as root, which is undesirable.
func (v *VMM) KillTTYProcess(containerName string, cmd string) error {
	process := strings.Split(cmd, " ")[0]
	resp, err := v.ContainerExec(containerName, fmt.Sprintf("ps -ef | awk '$8==\"%s\" {print $2}'", process), "vsoc-01")
	if err != nil {
		return err
	}
	pids := strings.Split(resp.outBuffer.String(), "\n")
	if len(pids) == 0 {
		log.Printf("Failed to kill process %s in container %s due to no matched pid found\n", process, containerName)
	}
	for _, pid := range pids {
		if pid != "" {
			_, err := v.ContainerExec(containerName, fmt.Sprintf("kill %s", pid), "root")
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
	cfIndex, err := v.GetVMInstanceNum(containerName)
	if err != nil {
		return &VMMError{"Failed to get VMInstanceNumber"}
	}
	vncPort := 6444 + cfIndex - 1
	wsPort := 6080 + cfIndex - 1
	resp, err := v.ContainerExec(containerName, fmt.Sprintf("websockify -D %d 127.0.0.1:%d --log-file websockify.log", wsPort, vncPort), "vsoc-01")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to start websockify, reason:" + resp.errBuffer.String()}
	}
	log.Println("websockify daemon started")
	return nil
}

func (v *VMM) startADBDaemon(containerName string) error {
	cfIndex, err := v.GetVMInstanceNum(containerName)
	if err != nil {
		return &VMMError{"Failed to get VMInstanceNumber"}
	}
	adbPort := 6520 + cfIndex - 1
	ip, err := v.getContainerIP(containerName)
	if err != nil {
		return &VMMError{"Failed to get container IP"}
	}
	resp, err := v.ContainerExec(containerName, fmt.Sprintf("adb connect %s:%d", ip, adbPort), "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to start adb daemon, reason" + resp.errBuffer.String()}
	}
	log.Printf("adb daemon connected to %s:%d", ip, adbPort)
	log.Println("adb stdout:" + resp.outBuffer.String())
	log.Println("adb stderr:" + resp.outBuffer.String())
	return nil
}

func (v *VMM) installTools(containerName string) error {
	resp, err := v.ContainerExec(containerName, "apt install -y -qq adb htop python3-pip iputils-ping less websockify", "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to apt install additional tools, reason:" + resp.errBuffer.String()}
	}
	resp, err = v.ContainerExec(containerName, "pip3 install frida-tools", "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to install python packages, reason:" + resp.errBuffer.String()}
	}
	return nil
}

func (v *VMM) InstallAPK(containerName string, apkFile string) error {
	f := path.Join(v.DevicesDir, containerName, apkFile)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		log.Printf("Abort installAPK because %s does not exist", f)
		return &VMMError{"Apk file does not exist"}
	}
	resp, err := v.ContainerExec(containerName, "adb install \"/data/"+apkFile+"\"", "vsoc-01")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"non-zero exit in installAPK: " + resp.errBuffer.String()}
	}
	return nil
}

func (v *VMM) getContainerNameByID(containerID string) (name string, err error) {
	containerJSON, err := v.Client.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return "", err
	}
	return containerJSON.Name[1:], nil
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

// Adapted from moby's exec implementation
// source: https://github.com/moby/moby/blob/master/integration/internal/container/exec.go
// Exec executes a command inside a container, returning the result
// containing stdout, stderr, and exit code. Note:
//  - this is a synchronous operation;
//  - cmd stdin is closed.
func (v *VMM) ContainerExec(containerName string, cmd string, user string) (ExecResult, error) {
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
	containerID, err := v.getContainerIDByName(containerName)
	if err != nil {
		return -1, err
	}
	containerJSON, err := v.Client.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return -1, err
	}
	// String representation of the container state
	// Can be one of "created", "running", "paused", "restarting", "removing", "exited", or "dead"
	if containerJSON.State.Status == "running" {
		// use grep "[x]xxx" technique to prevent grep itself from showing up in the ps result
		resp, err := v.ContainerExec(containerName, "ps aux|grep \"[l]aunch_cvd\"", "vsoc-01")
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

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func getRandomSequence(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func init() {
	rand.Seed(time.Now().UnixNano())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
