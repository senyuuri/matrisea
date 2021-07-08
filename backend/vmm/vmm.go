package vmm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

var (
	VMPrefix       = "matrisea-cvd-"                   // container name prefix
	DefaultNetwork = "cvd-bridge"                      // default docker network name
	CFImage        = "cuttlefish:latest"               // cuttlefish image
	ImageDir       = "/data/workspace/matrisea/images" // TODO read it from env
	WorkDir        = "/home/vsoc-01"                   // workdir inside container
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// A cuttlefish VM is essentially a crosvm process running in a docker container.
// Due to the one-one mapping, the word `VM` and `container` are sometimes used interchagably.
type VMM struct {
	Client *client.Client // Docker Engine client
}

type VMMError struct {
	msg string // description of error
}

func (e *VMMError) Error() string { return e.msg }

func NewVMM() *VMM {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	return &VMM{Client: cli}
}

// assume both the cuttlefish image and the default network exist on the host
func (v *VMM) CreateVM() error {
	ctx := context.Background()
	vmName := VMPrefix + randSeq(6)
	fmt.Printf(vmName)

	// create a new VM
	containerConfig := &container.Config{
		Image:    "cuttlefish",
		Hostname: vmName,
		Labels: map[string]string{ // for compatibility. Labels are used by android-cuttlefish CLI
			"cf_instance":     "0",
			"n_cf_instances":  "1",
			"vsock_guest_cid": "false",
		},
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
	}

	// attach the VM to the default bridge
	// the default bridge should have been created when the backend container started
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			DefaultNetwork: &network.EndpointSettings{},
		},
	}

	resp, err := v.Client.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, vmName)
	if err != nil {
		panic(err)
	}
	//log.Println(resp.ID)
	if err := v.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}
	return nil
}

// run launch_cvd inside of a running container
func (v *VMM) StartVM(containerName string, options string) error {
	fmt.Printf("StartVM: %s --> %s\n", containerName, options)
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		User:         "vsoc-01",
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/bash", "-c", "./bin/launch_cvd", options},
		//Cmd: []string{"/bin/sh", "-c", "ls -lah"},
		Tty: true,
	})
	if err != nil {
		panic(err)
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		panic(err)
	}

	defer hijackedResp.Close()
	// input of interactive shell
	// hijackedResp.Conn.Write([]byte("ls\r"))
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
		Cmd:          []string{"/bin/bash", "-c", "./bin/stop_cvd"},
		Tty:          true,
	})
	if err != nil {
		panic(err)
	}

	hijackedResp, err := v.Client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{Detach: false, Tty: true})
	if err != nil {
		panic(err)
	}

	defer hijackedResp.Close()
	scanner := bufio.NewScanner(hijackedResp.Conn)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	return nil
}

func (v *VMM) ListVM() ([]types.Container, error) {
	return nil, errors.New("TEST ERROR")
	containers, err := v.Client.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	vmList := []types.Container{}
	for _, container := range containers {
		if name := getCFContainerName(container); name != "" {
			vmList = append(vmList, container)
			fmt.Printf("%s %s %s\n", container.ID[:10], container.Names, container.Image)
		}
	}
	return vmList, nil
}

func (v *VMM) removeVM(containerID string) error {
	return v.Client.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{})
}

// remove all managed VMs
func (v *VMM) pruneVMs() {
	vmList, _ := v.ListVM()
	for _, vm := range vmList {
		err := v.Client.ContainerRemove(context.Background(), vm.ID, types.ContainerRemoveOptions{
			Force: true,
		})
		if err != nil {
			panic(err)
		}
		fmt.Printf("Removed %s\n", vm.ID)
	}
}

func (v *VMM) installAdeb() {
	// call
}

// srcPath must be a tar archive
func (v *VMM) copyToContainer(srcPath string, containerID string, dstPath string) error {
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

// copy aosp and cvd image into the container at container creation time
func (v *VMM) loadImages(containerID string) {
	if err := v.copyToContainer("/data/workspace/matrisea/images/cvd-host_package.tar", containerID, WorkDir); err != nil {
		log.Fatalf("%v", err)
	}
	// TODO unzip and tar before pass to vmm
	if err := v.copyToContainer("/data/workspace/matrisea/images/aosp_cf_x86_64_phone-img-7441291.tar", containerID, WorkDir); err != nil {
		log.Fatalf("%v", err)
	}
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
func getCFContainerName(container types.Container) string {
	for _, name := range container.Names {
		// docker container names all start with "/"
		prefix := "/" + VMPrefix
		if strings.HasPrefix(name, prefix) {
			return name[1:]
		}
	}
	return ""
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
