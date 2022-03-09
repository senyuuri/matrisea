/*
Package vmm implements a virtual machine manager that creates/starts/stops/destroies cuttlefish VM containers.

A `cuttlefish container` is essentially an Android Cuttlefish Virtual Device(CVD), running as a crosvm process in a docker container.
The android-cuttlefish image is defined in https://github.com/google/android-cuttlefish.

To avoid confusion between a `VM` and a `Container` in functions, here by convention
  - The word `VM` is used exclusively in exported functions for VM lifecycle management
  - The word `container` is used else where for direct interaction with the underlying containers

When setting up a new VM, the caller of should follow the call sequence below:
  1. Create a folder in $DATA/devices/your-device-name and upload device images (system + CVD images)
  2. VMCreate(your-device-name)
  3. VMVMPreBootSetup() to install packages and start daemons
  4. VMLoadFile() to copy the system image to the container's WorkDir
  5. VMUnzipImage() to unzip the system image
  6. VMLoadFile() to copy CVD image to the container's WorkDir
  7. VMStart()
*/
package vmm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
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
	"github.com/pkg/errors"
)

var (
	// DefaultNetwork: although the ideal design is to setup a new bridge network so as
	// to ensure better isolation between matrisea and other docker workloads on the same host.
	// However, this could introduce in-container DNS failure on Ubuntu 18.09+ as 18.09+ defaults
	// to systemd-resolved for host DNS management. (More details https://github.com/moby/moby/issues/38243)
	DefaultNetwork   = "bridge"        // use docker's default bridge
	CFImage          = "cuttlefish"    // cuttlefish image name
	HomeDir          = "/home/vsoc-01" // workdir in container
	HomeDirSizeLimit = 50              //soft disk quota for HomeDir
)

type VMM struct {
	Client      *client.Client // Docker Engine client
	DataDir     string
	DevicesDir  string
	DBDir       string
	UploadDir   string
	createMu    sync.Mutex    // Ensures only one CreateVM() call at a time
	CFPrefix    string        // Container name prefix
	BootTimeout time.Duration // Maximum waiting time for VMStart
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

func NewVMM(dataDir string) *VMM {
	v := NewVMMImpl(dataDir, "matrisea-cvd-", 120*time.Second)
	// watch for VMs in boot loops
	v.diskSheriff()
	return v
}

func NewVMMImpl(dataDir string, cfPrefix string, bootTimeout time.Duration) *VMM {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create a Docker API client. Reason: %v", err)
	}

	// populate initial data folders
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
				log.Fatalf("Failed to create folder %s. Reason: %v", f, err)
			}
		}
	}
	log.Printf("DATA_DIR=%s\n", dataDir)

	v := &VMM{
		Client:      cli,
		DataDir:     dataDir,
		DevicesDir:  devicesDir,
		DBDir:       dbDir,
		UploadDir:   uploadDir,
		CFPrefix:    cfPrefix,
		BootTimeout: bootTimeout,
	}
	return v
}

// VMCreate creates a new container and sets up the corresponding folders in DevicesDir.
func (v *VMM) VMCreate(deviceName string, cpu int, ram int, aospVersion string) (string, error) {
	ctx := context.Background()
	containerName := v.CFPrefix + deviceName

	// There will be a race condition on cfInstance if VMCreate() is called multiple times.
	// More specifically, findNextAvailableCFInstanceNumber() reads labels from existings containers.
	// If VMCreate() is called twice, both will get the same next available cf_instance as they both see the
	// same set of containers. By locking createMu, we ensure that one of the VMCreate() call
	// always complete first and finish creating a new container, so this new container will be counted towards the
	// next findNextAvailableCFInstanceNumber() call.
	v.createMu.Lock()
	defer v.createMu.Unlock()

	deviceDir := path.Join(v.DevicesDir, containerName)
	if _, err := os.Stat(deviceDir); os.IsNotExist(err) {
		if err = os.Mkdir(deviceDir, 0755); err != nil {
			return "", err
		}
	}

	// The next available index of cuttlefish VM. Always >= 1.
	// It is important for us to keep tracking of this index as cuttlefish use it to derive different
	// vsock ports for each instance in launch_cvd.
	cfInstance, err := v.getNextCFInstanceNumber()
	log.Printf("VMCreate: next available cf_instance %d", cfInstance)
	if err != nil {
		return "", errors.Wrap(err, "failed to get next cf_instance")
	}
	websockifyPort, err := nat.NewPort("tcp", strconv.Itoa(6080+cfInstance-1))
	if err != nil {
		return "", err
	}

	containerConfig := &container.Config{
		Image:    CFImage,
		Hostname: containerName,
		Labels: map[string]string{
			"cf_instance":               strconv.Itoa(cfInstance), //Used by android-cuttlefish CLI
			"n_cf_instances":            "1",                      //Used by android-cuttlefish CLI
			"vsock_guest_cid":           "true",                   //Used by android-cuttlefish CLI
			"matrisea_device_name":      deviceName,
			"matrisea_cpu":              strconv.Itoa(cpu),
			"matrisea_ram":              strconv.Itoa(ram),
			"matrisea_aosp_version":     aospVersion,
			"matrisea_tag_aosp_version": aospVersion, // Tags are for display only
		},
		Env: []string{
			"HOME=" + HomeDir,
		},
		ExposedPorts: nat.PortSet{
			websockifyPort: struct{}{},
		},
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
					// Expose websockify port
					HostIP:   "0.0.0.0",
					HostPort: strconv.Itoa(6080 + cfInstance - 1),
				},
			},
		},
	}

	// Attach the container to the default bridge, which should have been created by now.
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			DefaultNetwork: {},
		},
	}

	resp, err := v.Client.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", errors.Wrap(err, "ContainerCreate")
	}
	if err := v.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", errors.Wrap(err, "ContainerStart")
	}

	log.Printf("Created VM %s %s cf_instance/%d\n", containerName, resp.ID, cfInstance)

	return containerName, nil
}

// VMPreBootSetup installs necessary tools and start auxillary deamons in the container.
func (v *VMM) VMPreBootSetup(containerName string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	err := v.installTools(containerName)
	if err != nil {
		return errors.Wrap(err, "installTools")
	}
	err = v.startVNCProxy(containerName)
	if err != nil {
		return errors.Wrap(err, "startVNCProxy")
	}
	return nil
}

// VMStart runs launch_cvd in a running container.
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
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	cf_instance, err := v.getContainerCFInstanceNumber(containerName)
	if err != nil {
		return errors.Wrap(err, "getContainerCFInstanceNumber")
	}
	labels, err := v.getContainerLabels(containerName)
	if err != nil {
		return errors.Wrap(err, "getContainerLabels")
	}
	memory_gb, err := strconv.Atoi(labels["matrisea_ram"])
	if err != nil {
		return errors.Wrap(err, "read matrisea_ram label")
	}

	aospVersion, err := v.VMGetAOSPVersion(containerName)
	if err != nil {
		return errors.Wrap(err, "read AOSP version label")
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

	// Create an exec config in docker but do not run the command yet.
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
		return errors.Wrap(err, "docker: failed to create an exec config")
	}

	// Execute launch_cmd.
	aresp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return errors.Wrap(err, "docker: failed to execute/attach to launch_cvd")
	}
	defer aresp.Close()

	// ADB daemon needs to wait for the VM to boot in order to connect.
	// As we can't know for sure when the VM will start listening, our best chance to start ADB daemon is to
	// wait for VMStart to complete/timeout.
	defer func() {
		err = v.startADBDaemon(containerName)
		if err != nil {
			log.Printf("error: failed to startADBDaemon in %s. reason:%v", containerName, err)
		}
	}()

	// While the VM is booting, read the console output and wait for VIRTUAL_DEVICE_BOOT_COMPLETED message
	// to indicate a successful boot.
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
				log.Printf("VMStart (%s): success after %d\n", containerName, elapsed)
				return nil
			}
			return errors.New("VMStart failed as launch_cvd terminated abnormally")
		case <-time.After(v.BootTimeout):
			return errors.New("VMStart timeout")
		}
	}
	return nil
}

// VMStop kills launch_cvd process in the container.
func (v *VMM) VMStop(containerName string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
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
		return errors.Wrap(err, "failed to create an exec config in docker")
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return errors.Wrap(err, "failed to execute/attach to stop_cvd")
	}
	defer hijackedResp.Close()

	scanner := bufio.NewScanner(hijackedResp.Conn)
	output := ""
	for scanner.Scan() {
		line := scanner.Text()
		output = output + "\n" + line
		if strings.Contains(line, "Successful") {
			log.Printf("StopVM (%s): success\n", containerName)
			return nil
		}
	}
	return errors.New("failed to stop the VM. log: " + output)
}

// VMLoadFile copies a file from the host's srcPath to the container's HomeDir.
// If the file is a TAR archive, VMLoadFile will also untar it in the container.
func (v *VMM) VMLoadFile(containerName string, srcPath string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	return v.containerCopyFile(srcPath, containerName, HomeDir)
}

// VMUnzipImage unzips a zip file at the imageFile path of the container.
func (v *VMM) VMUnzipImage(containerName string, imageFile string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	match, _ := regexp.MatchString("^[a-zA-z0-9-_]+\\.zip$", imageFile)
	if !match {
		return errors.New("Failed to unzip due to invalid zip filename \"" + imageFile + "\"")
	}
	log.Printf("Unzip %s in container %s at %s", imageFile, containerName, HomeDir)
	_, err := v.containerExec(containerName, "unzip "+path.Join(HomeDir, imageFile), "vsoc-01")
	return errors.Wrap(err, "containerExec")
}

// VMRemove force removes a container, regardless of whether the VM is running.
func (v *VMM) VMRemove(containerName string) error {
	if _, err := v.isManagedContainer(containerName); err != nil {
		return err
	}
	containerID, err := v.getContainerIDByName(containerName)
	if err != nil {
		return errors.Wrap(err, "no containerID")
	}

	err = v.Client.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{
		Force: true,
		// required since /home/vsoc-01 is mounted as an anonymous volume
		RemoveVolumes: true,
	})
	if err != nil {
		return errors.Wrap(err, "docker: ContainerRemove")
	}
	err = os.RemoveAll(path.Join(v.DevicesDir, containerName))
	if err != nil {
		return err
	}
	return nil
}

// VMPrune removes all managed containers of the VMM instance. If there are more than one VMM running
// on the same host, VMPrune only removes containers with the VMM instance's CFPrefix.
func (v *VMM) VMPrune() {
	cfList, _ := v.listCuttlefishContainers()
	for _, c := range cfList {
		err := v.VMRemove(c.Names[0][1:])
		if err != nil {
			log.Printf("VMPrune (%s): failed. reason:%v\n", c.ID[:10], err)
		}
		log.Printf("VMPrune (%s): success\n", c.ID[:10])
	}
}

// VMList lists all managed containers of the VMM instance.
func (v *VMM) VMList() ([]VMItem, error) {
	cfList, err := v.listCuttlefishContainers()
	if err != nil {
		return nil, errors.Wrap(err, "listCuttlefishContainers")
	}
	resp := []VMItem{}
	for _, c := range cfList {
		status, err := v.getVMStatus(c)
		if err != nil {
			return nil, errors.Wrap(err, "getVMStatus")
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

// VMGetAOSPVersion reads the "matrisea_aosp_version" label of a container.
func (v *VMM) VMGetAOSPVersion(containerName string) (string, error) {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return "", err
	}
	containerJSON, err := v.getContainerJSON(containerName)
	if err != nil {
		return "", err
	}
	return containerJSON.Config.Labels["matrisea_aosp_version"], nil
}

// VMInstallAPK attempts to start an ADB daemon in the container and installs an apkFile on the VM.
// The apkFile should have been placed in the VM's deviceFolder. In the event that an ADB daemon
// is already running, calling startADBDaemon should have no effects.
func (v *VMM) VMInstallAPK(containerName string, apkFile string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	f := path.Join(v.DevicesDir, containerName, apkFile)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		log.Printf("VMInstallAPK (%s): abort installAPK because %s does not exist", containerName, f)
		return fmt.Errorf("apk file %s does not exist", apkFile)
	}
	// ADB daemon may have been terminated at this point so let's bring it up
	err := v.startADBDaemon(containerName)
	if err != nil {
		return errors.Wrap(err, "startADBDaemon")
	}
	resp, err := v.containerExec(containerName, "adb install \"/data/"+apkFile+"\"", "vsoc-01")
	if err != nil {
		return errors.Wrap(err, "adb install failed")
	}
	if resp.ExitCode != 0 {
		return errors.New("non-zero exit in installAPK: " + resp.errBuffer.String())
	}
	return nil
}

// ContainerAttachToTerminal starts a bash shell in the container and returns a bi-directional stream for the frontend to interact with.
// It's up to the caller to close the hijacked connection by calling types.HijackedResponse.Close.
// It's up to the caller to call KillTerminal() to kill the long running process at exit
func (v *VMM) ContainerAttachToTerminal(containerName string) (ir types.IDResponse, hr types.HijackedResponse, err error) {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return types.IDResponse{}, types.HijackedResponse{}, err
	}
	log.Printf("ExecAttachToTerminal %s\n", containerName)
	// TODO to do it properly, might need to get terminal dimensions from the front end
	// and dynamically adjust docker's tty dimensions
	// reference: https://github.com/xtermjs/xterm.js/issues/1359
	cmd := []string{"/bin/bash"}
	env := []string{"COLUMNS=`tput cols`", "LINES=`tput lines`"}
	return v.ContainerAttachToProcess(containerName, cmd, env)
}

// ContainerAttachToProcess starts a long running process with TTY and returns a bi-directional stream for the frontend to interact with.
// Notice:
//  - It's up to the caller to close the hijacked connection by calling types.HijackedResponse.Close.
//  - It's up to the caller to call KillTerminal() to kill the long running process at exit. (see reason below)
//
// Explanation: types.HijackedResponse.Close only calls HijackedResponse.Conn.Close() which leaves the process in the
// container to run forever. Moby's implementation of ContainerExecStart only terminates the process when either
// the context is Done or the attached stream returns EOF/error. In our use cases (e.g. bash/tail -f), the only possible
// way to terminate such long running processes by API is through context. However, if we trace ContainerExecAttach,
// Eventually we will end up at...
//
//  // github.com/moby/moby/api/server/router/container/exec.go#L132
//  // Now run the user process in container.
//  // Maybe we should we pass ctx here if we're not detaching?
//  s.backend.ContainerExecStart(context.Background(), ...)
//
// ... which always create a new context.Background(). Apparantly Moby team didn't implement the `maybe` part that allows
// context passing.
func (v *VMM) ContainerAttachToProcess(containerName string, cmd []string, env []string) (ID types.IDResponse, hr types.HijackedResponse, err error) {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return types.IDResponse{}, types.HijackedResponse{}, err
	}
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
		return types.IDResponse{}, types.HijackedResponse{}, errors.Wrap(err, "docker: failed to create an exec config")
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, ir.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return types.IDResponse{}, hijackedResp, errors.Wrap(err, "docker: failed to execute/attach to process")
	}
	return ir, hijackedResp, nil
}

// ContainerKillTerminal kills the bash process after use. To be called after done with the process created by ExecAttachToTerminal().
func (v *VMM) ContainerKillTerminal(containerName string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	return v.ContainerKillProcess(containerName, "/bin/bash")
}

// ContainerKillProcess kills all process in the given container with the given cmd. To be called after done with the process created by ExecAttachToTTYProcess().
//
// This is an ugly workaround since Moby's exec kill is long overdue (since 2014 https://github.com/moby/moby/pull/41548)
// Unfortunately we have to kill all pids of the same cmd since we can't get the specific terminal's pid in the container's
// pid namespace. This is because when creating a terminal in AttachToTerminal(), ContainerExecCreate only returns
// an execID that links to the spawned process's pid in the HOST pid namespace. We can't directly kill a host process unless
// we run the API server as root, which is undesirable.
func (v *VMM) ContainerKillProcess(containerName string, cmd string) error {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return err
	}
	process := strings.Split(cmd, " ")[0]
	resp, err := v.containerExec(containerName, fmt.Sprintf("ps -ef | awk '$8==\"%s\" {print $2}'", process), "vsoc-01")
	if err != nil {
		return errors.Wrap(err, "containerExec list process")
	}
	pids := strings.Split(resp.outBuffer.String(), "\n")
	if len(pids) == 0 {
		log.Printf("ContainerKillProcess (%s): 0 process found %s\n", containerName, process)
	}
	for _, pid := range pids {
		if pid != "" {
			_, err := v.containerExec(containerName, fmt.Sprintf("kill %s", pid), "root")
			if err != nil {
				// kill with best effort so just do logging
				log.Printf("ContainerKillProcess (%s): failed to kill %s;%s due to %v\n", containerName, pid, process, err)
				continue
			}
			log.Printf("ContainerKillProcess (%s): killed %s:%s", containerName, pid, process)
		}
	}
	return nil
}

// ContainerTerminalResize resizes the TTY size of a given execID
func (v *VMM) ContainerTerminalResize(execID string, lines uint, cols uint) error {
	return v.Client.ContainerExecResize(context.Background(), execID, types.ResizeOptions{Height: lines, Width: cols})
}

// ContainerListFiles gets a list of files in the given container's path
// Results are of the following format which each line represents a file/folder:
//
//  -rw-r--r--|vsoc-01|vsoc-01|65536|1645183964.5579601750|vbmeta.img
func (v *VMM) ContainerListFiles(containerName string, folder string) ([]string, error) {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return []string{}, err
	}
	cid, _ := v.getContainerIDByName(containerName)
	folder = path.Clean(folder)
	_, err := v.Client.ContainerStatPath(context.Background(), cid, folder)
	if err != nil {
		return []string{}, err
	}

	resp, err := v.containerExec(containerName, "find "+folder+" -maxdepth 1 -printf \"%M|%u|%g|%s|%A@|%P\n\" | sort -t '|' -k6", "vsoc-01")
	if err != nil || resp.ExitCode != 0 {
		return []string{}, errors.Wrap(err, "containerExec find")
	}
	lines := strings.Split(resp.outBuffer.String(), "\n")
	// remove the last empty line due to split
	return lines[:len(lines)-1], nil
}

// ContainaerFileExists checks if a given file/folder exist in the container.
func (v *VMM) ContainaerFileExists(containerName string, filePath string) error {
	cid, _ := v.getContainerIDByName(containerName)
	_, err := v.Client.ContainerStatPath(context.Background(), cid, filePath)
	return err
}

// ContainerReadFile gets a reader of a file in the container. As per Moby API's design, the file will be in TAR format so
// the caller should use tar.NewReader(reader) to obtain a corresponding tar reader.
// It is up to the caller to close the reader.
func (v *VMM) ContainerReadFile(containerName string, filePath string) (io.ReadCloser, error) {
	if err := v.isManagedRunningContainer(containerName); err != nil {
		return nil, err
	}
	id, err := v.getContainerIDByName(containerName)
	if err != nil {
		return nil, err
	}
	log.Printf("ContainerReadFile (%s): Copying file %s", containerName, filePath)
	// notice the API returns a reader for a TAR archive
	rc, _, err := v.Client.CopyFromContainer(context.TODO(), id, filePath)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

// getNextCFInstanceNumber returns the next smallest cf_instance number that have not been assigned.
func (v *VMM) getNextCFInstanceNumber() (int, error) {
	// Here we get all cuttlefish containers from the host's view, regardless of which VMM instance they belong to.
	//
	// listCuttlefishContainers is not used because it filter containers based on v.CFPrefix. In the case that
	// two VMMs are running on the same host (i.e. 1 for dev, 1 for go test), using listCuttlefishContainers will
	// create overlapped cf_instance numbers, which could lead to port conflicts.
	containerList, err := v.Client.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return -1, err
	}

	indexes := []int{}
	cfList := []types.Container{}
	for _, c := range containerList {
		if value, ok := c.Labels["cf_instance"]; ok {
			cfList = append(cfList, c)
			cf_idx, err := strconv.Atoi(value)
			if err != nil {
				return -1, err
			}
			indexes = append(indexes, cf_idx)
		}
	}
	sort.Ints(indexes)
	log.Printf("getNextCFInstanceNumber: num of existing cuttlefish containers: %d - %v\n", len(indexes), indexes)
	if len(indexes) == 0 {
		return 1, nil
	} else if indexes[len(indexes)-1] == len(cfList) {
		// if all assigned cf_instance numbers are continueous so far
		return len(cfList) + 1, nil
	} else {
		// find the smallest available cf_instance number
		i := 1
		for {
			if indexes[i-1] != i {
				return i, nil
			}
			i = i + 1
		}
	}
}

// getContainerCFInstanceNumber reads the cf_instance label of a container.
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

// startVNCProxy starts a websockify daemon in the container and listens to websocket-based VNC connection on the container port wsPort.
// startVNCProxy assumes the websockify binary exists in the container.
//
// When a cuttlefish VM is created with --start-vnc-server flag, /home/vsoc-01/bin/vnc_server starts to listen
// on 6444 of the `lo` interface. This vnc_server only supports RFB 3.x which isn't compatible with the websocket-based
// protocol of novnc.js. To allow the frontend to access the VNC stream inside of the container, we need to both
// translate RFB to websocket and to listen to a port on the container's `eth0` interface. websockify can do both.
func (v *VMM) startVNCProxy(containerName string) error {
	cfIndex, err := v.getContainerCFInstanceNumber(containerName)
	if err != nil {
		return errors.Wrap(err, "getContainerCFInstanceNumber")
	}
	vncPort := 6444 + cfIndex - 1
	wsPort := 6080 + cfIndex - 1
	resp, err := v.containerExec(containerName, fmt.Sprintf("websockify -D %d 127.0.0.1:%d --log-file websockify.log", wsPort, vncPort), "vsoc-01")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return errors.New("non-zero exit code in websockify. output:" + resp.errBuffer.String())
	}
	log.Printf("startVNCProxy (%s): websockify daemon started\n", containerName)
	return nil
}

// startADBDaemon starts an ADB daemon in the container and try connect to the VM.
// The function should be called when VM has booted up and started listening on the adb port.
// The function is safe to be called repeatedly as adb will ignore duplicated connect commands and return "already connected".
func (v *VMM) startADBDaemon(containerName string) error {
	cfIndex, err := v.getContainerCFInstanceNumber(containerName)
	if err != nil {
		return err
	}
	adbPort := 6520 + cfIndex - 1
	ip, err := v.getContainerIP(containerName)
	if err != nil {
		return err
	}
	resp, err := v.containerExec(containerName, fmt.Sprintf("adb connect %s:%d", ip, adbPort), "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return errors.New("non-zero exit code in adb daemon. stderr:" + resp.errBuffer.String())
	}
	log.Printf("startADBDaemon (%s): connected to %s:%d\n", containerName, ip, adbPort)
	log.Printf("startADBDaemon (%s): stdout:%s\n", containerName, resp.outBuffer.String())
	log.Printf("startADBDaemon (%s): stderr:%s\n", containerName, resp.outBuffer.String())
	return nil
}

func (v *VMM) installTools(containerName string) error {
	resp, err := v.containerExec(containerName, "apt install -y -qq adb git htop python3-pip iputils-ping less websockify", "root")
	if err != nil {
		return errors.Wrap(err, "failed to execute apt install")
	}
	if resp.ExitCode != 0 {
		return errors.New("Failed to apt install additional tools, reason:" + resp.errBuffer.String())
	}
	resp, err = v.containerExec(containerName, "pip3 install frida-tools", "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return errors.New("non-zero return when install python packages. reason:" + resp.errBuffer.String())
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
			prefix := "/" + v.CFPrefix
			if strings.HasPrefix(name, prefix) && strings.Contains(name, target) {
				return c.ID, nil
			}
		}
	}
	return "", errors.New("container not found")
}

// containerCopyFile copies a single file into the container.
// if srcPath isn't a .tar / tar.gz, it will be tar-ed in a temporary folder first
func (v *VMM) containerCopyFile(srcPath string, containerName string, dstPath string) error {
	start := time.Now()

	if strings.HasSuffix(srcPath, ".tar") || strings.HasSuffix(srcPath, ".tar.gz") {
		if err := v.containerCopyTarFile(srcPath, containerName, dstPath); err != nil {
			return errors.Wrap(err, "containerCopyTarFile")
		}
	}

	tmpdir, err := ioutil.TempDir("", "matrisea")
	if err != nil {
		return errors.Wrap(err, "cannot create tmp dir")
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
		return errors.Wrap(err, "error during tar")
	}

	archive := tmpdir + "/" + srcFile + ".tar"
	if err = v.containerCopyTarFile(archive, containerName, dstPath); err != nil {
		return errors.Wrap(err, "containerCopyTarFile")
	}

	elapsed := time.Since(start)
	log.Printf("containerCopyFile (%s): src:%s dst:%s cost:%s\n", containerName, srcPath, dstPath, elapsed)
	return nil
}

// containerCopyTarFile is a wrapper function of docker's CopyToContainer API where the srcPath must be a tar file
// The API will fail silently if srcPath isn't a tar.
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
		return errors.Wrap(err, "docker: CopyToContainer")
	}
	return nil
}

func (v *VMM) containerExec(containerName string, cmd string, user string) (ExecResult, error) {
	return v.containerExecWithContext(context.Background(), containerName, cmd, user)
}

// Execute a command in a container and return the result
// containing stdout, stderr, and exit code. Note:
//  - The function is synchronous
//  - stdin is closed
//
// Adapted from moby's exec implementation
// https://github.com/moby/moby/blob/master/integration/internal/container/exec.go
func (v *VMM) containerExecWithContext(ctx context.Context, containerName string, cmd string, user string) (ExecResult, error) {
	execConfig := types.ExecConfig{
		User:         user,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-c", cmd},
	}
	cresp, err := v.Client.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return ExecResult{}, errors.Wrap(err, "docker: failed to create an exec config")
	}

	execID := cresp.ID
	aresp, err := v.Client.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return ExecResult{}, errors.Wrap(err, "docker: failed to execute/attach to "+cmd)
	}
	defer aresp.Close()

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
	case <-ctx.Done():
		return ExecResult{}, errors.Wrap(ctx.Err(), "context done")
	}

	iresp, err := v.Client.ContainerExecInspect(ctx, execID)
	if err != nil {
		return ExecResult{}, errors.Wrap(err, "docker: ContainerExecInspect")
	}
	// Let the caller to handler non-zero exit code.
	return ExecResult{ExitCode: iresp.ExitCode, outBuffer: &outBuf, errBuffer: &errBuf}, nil
}

// listCuttlefishContainers gets a list of managed containers of the VMM instance.
func (v *VMM) listCuttlefishContainers() ([]types.Container, error) {
	containers, err := v.Client.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	cflist := []types.Container{}
	for _, c := range containers {
		if strings.HasPrefix(c.Names[0], "/"+v.CFPrefix) {
			cflist = append(cflist, c)
		}
	}
	return cflist, nil
}

type ExecChannelResult struct {
	resp ExecResult
	err  error
}

// getVMStatus derives a VMStatus, with best-effort, from the container's status and whether launch_cvd is running in the container.
// Due to the concurrency design of the underlying Moby API, if a given container is locked (busy with other requests),
// the returned VMStatus might not accurately reflect the actual status of launch_cvd.
func (v *VMM) getVMStatus(c types.Container) (VMStatus, error) {
	// Create a context that will be canceled in 300ms
	//
	// Many Moby APIs acquires a per-container lock during execution. For example, in Daemon.containerCopy (used by VMM.containerCopyFile):
	// https://github.com/moby/moby/blob/eb9e42a09ee123af1d95bf7d46dd738258fa2109/daemon/archive.go#L390
	// If one of such APIs runs for a long time (e.g. copy a large file from a container), the container's lock
	// can be held long enough that blocks subsequent API calls. Unfortunately, getVMStatus is the one that gets
	// blocked the most because it's used by VMList, one of the hottest code path that gets called by every client
	// every 5 seconds. Hence, to avoid waiting for a container's lock indefinitely, we only try query a container's
	// process list (`ps aux`) for a limited amount of time
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	containerName := c.Names[0][1:]
	// When a container is up, c.Status looks like "Up 2 days"
	if strings.HasPrefix(c.Status, "Up") {
		ch := make(chan ExecChannelResult, 1)
		go func() {
			// use grep "[x]xxx" technique to prevent grep itself from showing up in the ps result
			resp, err := v.containerExecWithContext(ctx, containerName, "ps aux|grep \"[l]aunch_cvd\"", "vsoc-01")
			ch <- ExecChannelResult{resp, err}
		}()

		select {
		case <-ctx.Done():
			// The container's lock is held by others, probably busy doing other tasks
			// The container is certainly running but we're not sure about launch_cvd
			log.Printf("getVMStatus (%s): Timeout warning. Defaulting to VMReady\n", containerName)
			return VMReady, nil
		case execResult := <-ch:
			if execResult.err != nil {
				fmt.Printf("getVMStatus failed to list processes: %v\n", execResult.err)
				return -1, errors.Wrap(execResult.err, "getVMStatus failed to top")
			}
			if strings.Contains(execResult.resp.outBuffer.String(), "launch_cvd") {
				return VMRunning, nil
			}
			return VMReady, nil
		}
	}
	return VMContainerError, nil
}

// isManagedRunningContainer checks if a given container exists && is managed by the VMM instance && is running
func (v *VMM) isManagedRunningContainer(containerName string) error {
	cjson, err := v.isManagedContainer(containerName)
	if err != nil {
		return err
	}
	if cjson.State.Status != "running" {
		return fmt.Errorf("invalid container: container not running")
	}
	return nil
}

// isManagedContainer checks if a given container exists && is managed by the VMM instance
func (v *VMM) isManagedContainer(containerName string) (types.ContainerJSON, error) {
	cid, err := v.getContainerIDByName(containerName)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("invalid contaienr name: %w", err)
	}
	cjson, err := v.Client.ContainerInspect(context.Background(), cid)
	if err != nil {
		return types.ContainerJSON{}, fmt.Errorf("invalid container, error reading container JSON: %w", err)
	}
	if !strings.HasPrefix(cjson.Name, "/"+v.CFPrefix) {
		return types.ContainerJSON{}, errors.New("invalid container: non-cuttlefish found")
	}
	return cjson, nil
}

// diskSheriff periodically checks all managed containers' volume usage (/home/vsoc-01) and stops the VM (i.e. stop launch_cvd)
// if the volume size has exceeded the limit.
//
// This is because cuttlefish forces restart on all crashed subprocesses and there is no option to override such behavior.
// If a VM has crashed and entered an unrecovable state, launch_cvd will enter a boot loop, generates large amount of launcher log,
// and eventually fill up the entire disk. Docker's disk quota feature (via --storage-opt) won't help in this case as
// the feature relies docker's overlayfs2 driver to use a non-default xfs backing fs.
// To prevent this rare yet devastating scenario a.k.a. device entering a boot loop and left running forever, diskShriff() runs
// periodically to check if the container's /home/vsoc-01 volume has exceeded a given limit. If true, VMStop is called against the VM.
func (v *VMM) diskSheriff() {
	log.Println("DiskSheriff started")
	go func() {
		for {
			containers, err := v.listCuttlefishContainers()
			if err != nil {
				log.Printf("DiskSheriff: failed to list containers. error: %v\n", err)
			}

			for _, c := range containers {
				containerName := c.Names[0][1:]
				// It's okay if getVMStatu is busy waiting for a lock. Let other request to finish first
				status, err := v.getVMStatus(c)
				if err != nil {
					log.Printf("DiskSheriff: failed to get VMStatus error: %v\n", err)
				}
				if status == VMRunning {
					volSize, err := v.getContainerHomeDirUsage(containerName)
					if err != nil {
						log.Printf("DiskSheriff: failed to get volume usage. error: %v\n", err)
					}
					// fmt.Printf("DiskSheriff,%s,%f\n", containerName, float64(volSize)/(math.Pow(1024, 3)))
					// TODO read limit from container labels
					if float64(volSize)/(math.Pow(1024, 3)) > float64(HomeDirSizeLimit) {
						log.Printf("DiskSheriff: VM %s has exceeded disk limit, probably in a boot loop, stopping now\n", containerName)
						if err := v.VMStop(containerName); err != nil {
							log.Printf("DiskSheriff: failed to stop VM %s. error %v\n", containerName, err)
						}
					}
				}
			}
			time.Sleep(30 * time.Second)
		}
	}()
}

func (v *VMM) getContainerHomeDirUsage(containerName string) (int64, error) {
	// Volume.UsageData.Size is only populates by DiskUsage()
	du, err := v.Client.DiskUsage(context.Background())
	if err != nil {
		return 0, err
	}
	c, err := v.getContainerJSON(containerName)
	if err != nil {
		return 0, err
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
	return 0, fmt.Errorf("couldn't find %s volume in container %s", HomeDir, containerName)
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
