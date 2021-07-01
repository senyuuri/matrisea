package vmm

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

var (
	VMPrefix       = "matrisea-cvd-"                  // container name prefix
	DefaultNetwork = "cvd-bridge"                     // default docker network name
	CFImage        = "cuttlefish:latest"              // cuttlefish image
	ImagePath      = "/home/senyuuri/matrisea/images" // TODO read it from env
)

// Virtual machine manager that create/start/stop/destroy cuttlefish VMs
// A cuttlefish VM is essentially a crosvm process running in a docker container.
// Due to the one-one mapping, the word `VM` and `container` are sometimes used interchagably.
type Vmm struct {
	Client *client.Client // Docker Engine client
}

type VmmError struct {
	msg string // description of error
}

func (e *VmmError) Error() string { return e.msg }

func NewVmm() *Vmm {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	return &Vmm{Client: cli}
}

// assume both the cuttlefish image and the default network exist on the host
func (v *Vmm) CreateVM() error {
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
func (v *Vmm) StartVM(containerName string, options string) error {
	fmt.Printf("StartVM: %s --> %s\n", containerName, options)
	ctx := context.Background()
	resp, err := v.Client.ContainerExecCreate(ctx, containerName, types.ExecConfig{
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
func (v *Vmm) StopVM() {
	// call
}

func (v *Vmm) ListVM() []types.Container {
	containers, err := v.Client.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		panic(err)
	}
	vmList := []types.Container{}
	for _, container := range containers {
		if name := getCFContainerName(container); name != "" {
			vmList = append(vmList, container)
			fmt.Printf("%s %s %s\n", container.ID[:10], container.Names, container.Image)
		}
	}
	return vmList
}

func (v *Vmm) removeVM(containerID string) error {
	return v.Client.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{})
}

// remove all managed VMs
func (v *Vmm) pruneVMs() {
	vmList := v.ListVM()
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

func (v *Vmm) InstallAdeb() {
	// call
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
			return name
		}
	}
	return ""
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
