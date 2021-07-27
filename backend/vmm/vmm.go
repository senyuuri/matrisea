package vmm

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// for DefaultNetwork: although the ideal design is to setup a new bridge network so as
// to ensure better isolation between matrisea and other docker workloads on the same host.
// However, this could introduce in-container DNS failure on Ubuntu 18.09+ as 18.09+ defaults
// to systemd-resolved for host DNS management.
//
// More details: https://github.com/moby/moby/issues/38243
var (
	VMPrefix       = "matrisea-cvd-"                   // container name prefix
	DefaultNetwork = "bridge"                          // use docker's default bridge
	CFImage        = "cuttlefish:latest"               // cuttlefish image
	ImageDir       = "/data/workspace/matrisea/images" // TODO read it from env
	WorkDir        = "/home/vsoc-01"                   // workdir inside container
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// A cuttlefish VM is essentially a crosvm process running in a docker container.
// Due to the one-one mapping, the word `VM` and `container` are sometimes used interchagably.
type VMM struct {
	Client    *client.Client // Docker Engine client
	UploadDir string         // path of uploaded files
	ImageDir  string         // path of device images
}

type VMMError struct {
	msg string // description of error
}

func (e *VMMError) Error() string { return e.msg }

func NewVMM(imageDir string, uploadDir string) (*VMM, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	log.Printf("UPLOAD_DIR=%s\n", uploadDir)
	log.Printf("IMAGE_DIR=%s\n", imageDir)

	return &VMM{
		Client:    cli,
		UploadDir: uploadDir,
		ImageDir:  imageDir,
	}, nil
}

// assume both the cuttlefish image and the default network exist on the host
func (v *VMM) CreateVM() (name string, err error) {
	ctx := context.Background()
	vmName := VMPrefix + randSeq(6)

	// dedicate a folder on the host for storing VM images
	imageDir := v.createImageFolder(vmName)

	// create a new VM
	containerConfig := &container.Config{
		Image:    "cuttlefish",
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
				Target:   WorkDir,
				ReadOnly: false,
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
func (v *VMM) StartVM(containerName string, options string) error {
	log.Printf("StartVM %s with options: %s\n", containerName, options)
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{WorkDir + "/bin/launch_cvd", "--nostart_webrtc", "--start_vnc_server"},
		Tty:          true,
	})
	if err != nil {
		return err
	}

	// launch_cvd only starts running when a tty is attached
	hijackedResp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		return err
	}

	// TODO check return code
	defer hijackedResp.Close()
	// input of interactive shell
	hijackedResp.Conn.Write([]byte("ls\r"))
	scanner := bufio.NewScanner(hijackedResp.Conn)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	return nil
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
	// input of interactive shell
	// hijackedResp.Conn.Write([]byte("ls\r"))
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

func (v *VMM) ListVM() ([]types.Container, error) {
	containers, err := v.Client.ContainerList(context.Background(), types.ContainerListOptions{All: true})
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

	if err = v.deleteImageFolder(containerName); err != nil {
		return err
	}
	return nil
}

// remove all managed VMs
func (v *VMM) PruneVMs() {
	log.Println("PruneVMs called")
	vmList, _ := v.ListVM()
	for _, vm := range vmList {
		err := v.RemoveVM(vm.ID)
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
	vmList, err := v.ListVM()
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

// srcPath must be a tar archive
func (v *VMM) CopyToContainer(srcPath string, containerID string, dstPath string) error {
	log.Printf("Load file into container %s:\n", containerID)
	log.Printf("src: %s\n", srcPath)
	log.Printf("dst: %s\n", dstPath)
	// TODO support loding non-tar files and directories. check if src is not a tar file, tar first
	archive, err := os.Open(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("Failed to load a file into the container because the file does not exist: %s\n", srcPath)
		}
		fmt.Println(err)
	}
	defer archive.Close()

	opts := types.CopyToContainerOptions{}
	if err := v.Client.CopyToContainer(context.TODO(), containerID, dstPath, bufio.NewReader(archive), opts); err != nil {
		log.Fatalf("%v", err)
	}
	return nil
}

// create a folder on the host and bind mount it to a given container
// preferred over CopyToContainer() as it saves both time and space
// (for keeping two copies of images larger than 10GB)
func (v *VMM) createImageFolder(containerName string) string {
	path := path.Join(v.ImageDir, containerName)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Created image folder %s for container %s\n", path, containerName)
	return path
}

func (v *VMM) deleteImageFolder(containerName string) error {
	path := path.Join(v.ImageDir, containerName)
	err := os.RemoveAll(path)
	if err != nil {
		return err
	}
	log.Printf("Deleted image folder %s for container %s\n", path, containerName)
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
	imageDir := path.Join(v.ImageDir, containerName)
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
