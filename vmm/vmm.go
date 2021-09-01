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
	VMPrefix       = "matrisea-cvd-" // container name prefix
	DefaultNetwork = "bridge"        // use docker's default bridge
	CFImage        = "cuttlefish"    // cuttlefish image name
	WorkDir        = "/home/vsoc-01" // workdir in container
	ROImageDir     = "/root/images"  //read-only device image dir in container
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// A cuttlefish VM is essentially a crosvm process running in a docker container.
// Due to the one-one mapping, the word `VM` and `container` are sometimes used interchagably.
//
// Caller of VMM is responsible to follow the call sequence when setting up a VM:
//   1. Create a folder in $DATA/images/your-device-name and upload device images into it
//   2. VMCreate()
//   3. CopyToContainer() to load device images into WorkDir (in container)
//   4. VMStart()
type VMM struct {
	Client    *client.Client // Docker Engine client
	DataDir   string
	ImagesDir string
	VmsDir    string
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
	vmsDir := path.Join(dataDir, "vms")
	folders := []string{
		dataDir,
		imagesDir,
		vmsDir,
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
		VmsDir:    vmsDir,
	}, nil
}

// assume both the cuttlefish image and the default network exist on the host
// a baseDevice represents a set of default images to be mounted to the container
func (v *VMM) CreateVM(baseDevice string) (name string, err error) {
	ctx := context.Background()
	vmName := VMPrefix + randSeq(6)

	// dedicate a folder on the host for storing VM data
	_, err = v.createVMFolder(vmName)
	if err != nil {
		return "", err
	}

	// create a new VM
	containerConfig := &container.Config{
		Image:    CFImage,
		Hostname: vmName,
		Labels: map[string]string{ // for compatibility. Labels are used by android-cuttlefish CLI
			"cf_instance":     "0",
			"n_cf_instances":  "1",
			"vsock_guest_cid": "false",
		},
		Env: []string{
			"HOME=/home/vsoc-01",
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
			{
				Type:     mount.TypeBind,
				Source:   imageDir,
				Target:   ROImageDir,
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

	// attach the VM to the default bridge
	// the default bridge should have been created when the backend container started
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			DefaultNetwork: {},
		},
	}

	resp, err := v.Client.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, vmName)
	if err != nil {
		return "", err
	}
	if err := v.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", err
	}
	log.Printf("Created VM %s %s\n", vmName, resp.ID)

	return vmName, nil
}

// run launch_cvd inside of a running container
// notice StartVM() doesn't check for succeesful VM boot as it could take a long time
func (v *VMM) StartVM(containerName string, options string) (execID string, err error) {
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{WorkDir + "/bin/launch_cvd", "--nostart_webrtc", "--start_vnc_server"},
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
func (v *VMM) StopVM(containerName string) error {
	fmt.Printf("StopVM: %s\n", containerName)
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{WorkDir + "/bin/stop_cvd"},
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

func (v *VMM) listVM(ctx context.Context) ([]types.Container, error) {
	containers, err := v.Client.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	vmList := []types.Container{}
	for _, container := range containers {
		if isCuttlefishContainer(container) {
			vmList = append(vmList, container)
		}
	}
	return vmList, nil
}

func (v *VMM) RemoveVM(containerName string) error {
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

	if err = v.deleteVMFolder(containerName); err != nil {
		return err
	}
	return nil
}

// remove all managed VMs
func (v *VMM) PruneVMs() {
	log.Println("PruneVMs called")
	vmList, _ := v.listVM(context.Background())
	for _, vm := range vmList {
		err := v.RemoveVM(vm.Names[0][1:])
		if err != nil {
			panic(err)
		}
		log.Printf("Removed VM %s\n", vm.ID[:10])
	}
}

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
	vmList, err := v.listVM(context.Background())
	if err != nil {
		return "", err
	}
	for _, vm := range vmList {
		for _, name := range vm.Names {
			// docker container names all start with "/"
			prefix := "/" + VMPrefix
			if strings.HasPrefix(name, prefix) && strings.Contains(name, target) {
				return vm.ID, nil
			}
		}
	}
	return "", nil
}

// copy a single file into the container
// if srcPath isn't a tar, it will be tar-ed in a temporary folder first
func (v *VMM) CopyToContainer(srcPath string, containerName string, dstPath string) error {
	log.Printf("Copy file into container %s:\n", containerName)
	log.Printf("  src: %s\n", srcPath)
	log.Printf("  dst: %s\n", dstPath)
	start := time.Now()

	if strings.HasSuffix(srcPath, ".tar") {
		v.copyToContainer(srcPath, containerName, dstPath)
	}

	tmpdir, err := ioutil.TempDir("", "matrisea")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)
	_, srcFile := filepath.Split(srcPath)

	cmdStr := fmt.Sprintf("tar -cvf %s/%s.tar %s", tmpdir, srcFile, srcPath)

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
	if err = v.copyToContainer(archive, containerName, dstPath); err != nil {
		return err
	}

	elapsed := time.Since(start)
	log.Printf("  Copy completed in %s\n", elapsed)
	return nil
}

// wrapper function on docker's CopyToContainer API where the srcPath must be a tar file
// The API will fail silently if srcPath isn't a tar
func (v *VMM) copyToContainer(srcPath string, containerName string, dstPath string) error {
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
func (v *VMM) ContainerExec(containerName string, cmd string) (ExecResult, error) {
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

// create a VM specific data folder and initialise 3 subfolders for overlayFS,
// namely `upper`, `work`, and `merge`
func (v *VMM) createVMFolder(containerName string) (string, error) {
	parent := path.Join(v.VmsDir, containerName)

	err := os.Mkdir(parent, 0755)
	if err != nil {
		log.Println(err)
		return "", err
	}
	log.Printf("Created data folder %s for container %s\n", parent, containerName)
	return parent, nil
}

func (v *VMM) deleteVMFolder(containerName string) error {
	path := path.Join(v.VmsDir, containerName)
	err := os.RemoveAll(path)
	if err != nil {
		return err
	}
	log.Printf("Deleted data folder %s for container %s\n", path, containerName)
	return nil
}

// copy aosp and cvd image into the container's image folder
// expect aosp image to be .zip and cvd image to be .tar, as per android CI's default packaing
func (v *VMM) LoadImages(containerName string, aosp_zip string, cvd_tar string) error {
	if _, err := os.Stat(aosp_zip); os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(cvd_tar); os.IsNotExist(err) {
		return err
	}
	imageDir := path.Join(v.ImagesDir, containerName)
	cmd := exec.Command("unzip", aosp_zip, "-d", imageDir)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Fatalf("Unzip failed with error code %d", exitError.ExitCode())
		}
	}
	cmd = exec.Command("tar", "-C", imageDir, "-xzf", cvd_tar)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			log.Fatalf("Untar failed with error code %d", exitError.ExitCode())
		}
	}
	return nil
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// get the VMPrefix name of a cuttlefish container
// if it is not a cuttlefish container, return ""
func isCuttlefishContainer(container types.Container) bool {
	for _, name := range container.Names {
		// docker container names all start with "/"
		prefix := "/" + VMPrefix
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func init() {
	rand.Seed(time.Now().UnixNano())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
