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
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// In Matrisea, a cuttlefish VM is essentially a crosvm process running in a docker container.
// To avoid confusion between a `VM` and a `Container`, here by convention
//   - The word `VM` is used exclusively in exported functions for VM lifecycle management
//   - The word `container` is used else where for direct interaction with the underlying containers
//
// Caller of VMM is responsible to follow the call sequence when setting up a VM:
//   1. Create a folder in $DATA/images/your-device-name and upload device images into it
//   2. VMCreate(your-device-name)
//   3. VMLoadFile() to copy images to the container's WorkDir
//   4. VMStart()
type VMM struct {
	Client    *client.Client // Docker Engine client
	DataDir   string
	ImagesDir string
}

type VMMError struct {
	msg string // description of error
}

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
	imagesDir := path.Join(dataDir, "images")
	folders := []string{
		dataDir,
		imagesDir,
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
		Client:    cli,
		DataDir:   dataDir,
		ImagesDir: imagesDir,
	}, nil
}

// assume both the cuttlefish image and the default network exist on the host
// a baseDevice represents a set of default images to be mounted to the container
func (v *VMM) VMCreate(baseDevice string) (name string, err error) {
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
		},
		Env: []string{
			"HOME=" + HomeDir,
		},
		// TODO disable VNC port binding in production
		// ExposedPorts: nat.PortSet{
		// 	"6444/tcp": struct{}{},
		// },
	}

	imageDir := path.Join(v.ImagesDir, baseDevice)
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

	// cmd only starts after ContainerExecAttach
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
		}
		outputDone <- 0
	}()

	select {
	case <-outputDone:
		log.Println("EOF")
	case <-time.After(5 * time.Second):
		// TODO test what happens when VM start successfully but timeout
		log.Println("Timeout")
	}

	iresp, err := v.Client.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return "", err
	}
	if iresp.ExitCode != 0 {
		err_msg := fmt.Sprintf("VM exited with non-zero error code: %d", iresp.ExitCode)
		return "", &VMMError{msg: err_msg}
	}
	log.Printf("VM %s started\n", containerName)
	return "", nil
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

	// TODO check if crosvm process has been stopped
	err = v.Client.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil {
		return err
	}

	return nil
}

// remove all managed VMs
func (v *VMM) VMPrune() {
	log.Println("PruneVMs called")
	cfList, _ := v.getCuttlefishContainers(context.Background())
	for _, c := range cfList {
		err := v.VMRemove(c.Names[0][1:])
		if err != nil {
			panic(err)
		}
		log.Printf("Removed VM %s\n", c.ID[:10])
	}
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
	cfList, err := v.getCuttlefishContainers(context.Background())
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

func (v *VMM) getCuttlefishContainers(ctx context.Context) ([]types.Container, error) {
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
func (v *VMM) containerExec(containerName string, cmd string) (ExecResult, error) {
	log.Printf("ContainerExec %s: %s\n", containerName, cmd)
	ctx := context.Background()
	// prepare exec
	execConfig := types.ExecConfig{
		User:         "vsoc-01",
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

	return ExecResult{ExitCode: iresp.ExitCode, outBuffer: &outBuf, errBuffer: &errBuf}, nil
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
