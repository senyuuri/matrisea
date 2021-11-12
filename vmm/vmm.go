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
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
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
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Created string   `json:"created"` // unix timestamp
	Device  string   `json:"device"`
	IP      string   `json:"ip"`
	Status  VMStatus `json:"status"`
	Tags    []string `json:"tags"`
}

type VMs []VMItem

type VMStatus int

const (
	VMContainerReady   VMStatus = iota // container is up but crosvm not running
	VMRunning          VMStatus = iota // crosvm is running
	VMContainerStopped VMStatus = iota // container is stopped but this shouldn't happen
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
func (v *VMM) VMCreate(deviceName string) (name string, err error) {
	ctx := context.Background()
	containerName := CFPrefix + getRandomSequence(6)

	// create a new VM
	containerConfig := &container.Config{
		Image:    CFImage,
		Hostname: containerName,
		Labels: map[string]string{ // for compatibility. Labels are used by android-cuttlefish CLI
			"cf_instance":     "0",
			"n_cf_instances":  "1",
			"vsock_guest_cid": "false",
			"matrisea_device": deviceName,
		},
		Env: []string{
			"HOME=" + HomeDir,
		},
		// TODO disable VNC port binding in production
		// ExposedPorts: nat.PortSet{
		// 	"6444/tcp": struct{}{},
		// },
	}

	imageDir := path.Join(v.DevicesDir, deviceName)
	if _, err := os.Stat(imageDir); os.IsNotExist(err) {
		return "", err
	}

	hostConfig := &container.HostConfig{
		Privileged: true,
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   "/sys/fs/cgroup",
				Target:   "/sys/fs/cgroup",
				ReadOnly: true,
			},
		},
		// TODO disable VNC port binding in production
		// PortBindings: nat.PortMap{
		// 	"6444/tcp": []nat.PortBinding{
		// 		{
		// 			HostIP:   "0.0.0.0",
		// 			HostPort: "6444",
		// 		},
		// 	},
		// },
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
	log.Printf("Created VM %s %s\n", containerName, resp.ID)

	return containerName, nil
}

// run launch_cvd inside of a running container
// notice VMStart() doesn't check for succeesful VM boot as it could take a long time
func (v *VMM) VMStart(containerName string, options string) (execID string, err error) {
	start := time.Now()
	// start auxillary deamons
	err = v.startVNCProxy(containerName)
	if err != nil {
		return "", err
	}

	// start cuttlefish device
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{HomeDir + "/bin/launch_cvd", "--nostart_webrtc", "--start_vnc_server"},
		Tty:          true,
	})
	if err != nil {
		return "", err
	}

	// cmd only get executed after ContainerExecAttach
	aresp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return "", err
	}
	defer aresp.Close()

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
			return "", nil
		}
		return "", &VMMError{msg: "VMStart EOF while reading output"}
	case <-time.After(TimeoutVMStart):
		return "", &VMMError{msg: "VMStart timeout"}
	}
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

		resp = append(resp, VMItem{
			ID:      c.ID,
			Name:    containerName,
			Created: strconv.FormatInt(c.Created, 10),
			Device:  c.Labels["matrisea_device_template"],
			IP:      c.NetworkSettings.Networks[DefaultNetwork].IPAddress,
			Status:  status,
			Tags:    []string{},
		})
	}
	return resp, nil
}

// returns a bi-directional stream for the frontend to interact with a container's shell
func (v *VMM) AttachToTerminal(containerName string) (hr types.HijackedResponse, err error) {
	log.Printf("Request to attach to container terminal %s\n", containerName)
	ctx := context.Background()
	ir, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/bash"},
		Tty:          true,
	})
	if err != nil {
		return
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, ir.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return hijackedResp, err
	}
	return hijackedResp, nil
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
	resp, err := v.ContainerExec(containerName, "apt install -y -qq websockify", "root")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to install websockify"}
	}
	resp, err = v.ContainerExec(containerName, "websockify -D 6080 127.0.0.1:6444", "vsoc-01")
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return &VMMError{"Failed to start websockify"}
	}
	log.Println("websockify daemon started")
	return nil
}

func (v *VMM) installAdeb() {
	// call
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
	log.Printf("ContainerExec %s: %s\n", containerName, cmd)
	start := time.Now()
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

	elapsed := time.Since(start)
	log.Printf("  ExitCode: %d\n", iresp.ExitCode)
	log.Printf("  stdout: %s\n", outBuf.String())
	log.Printf("  stderr: %s\n", errBuf.String())
	log.Printf("  ContainerExec completed in %s\n", elapsed)
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
	// String representation of the container state. Can be one of "created", "running", "paused", "restarting", "removing", "exited", or "dead"
	if containerJSON.State.Status == "running" {
		// use grep "[x]xxx" technique to prevent grep itself from showing up in the ps result
		resp, err := v.ContainerExec(containerName, "ps aux|grep \"[l]aunch_cvd\"", "vsoc-01")
		if err != nil {
			return -1, err
		}
		if strings.Contains(resp.outBuffer.String(), "launch_cvd") {
			return VMRunning, nil
		}
		return VMContainerReady, nil
	}
	log.Printf("Unexpected status %s of container %s\n", containerJSON.State.Status, containerName)
	return VMContainerStopped, nil
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
