package main

import (
	"context"
	"fmt"
	"log"
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
	VMPrefix       = "matrisea-cvd-" // container name prefix
	DefaultNetwork = "cvd-bridge"    // default docker network name
)

// A virtual machine manager that create/start/stop/destroy a cuttlefish VM
type Vmm struct {
	Client *client.Client // Docker Engine client
}

func NewVmm() *Vmm {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	return &Vmm{Client: cli}
}

func (v *Vmm) CreateVM() error {
	// check if cuttlefish image is ready
	isReady, err := v.isCuttlefishImageReady()
	if !isReady {
		panic("Cannot find docker image cuttlefish:latest. Try to run setup.sh again")
	}
	if err != nil {
		panic(err)
	}
	rand.Seed(time.Now().UnixNano())
	vmName := VMPrefix + randSeq(6)
	fmt.Printf(vmName)

	// create a new VM
	containerConfig := &container.Config{
		Image:    "alpine",
		Hostname: vmName,
		Labels: map[string]string{ // set labels used by android-cuttlefish cli for compatibility
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
	// configure the new VM to attach to the default bridge
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"test-bridge": &network.EndpointSettings{},
		},
	}
	resp, err := v.Client.ContainerCreate(context.Background(), containerConfig, hostConfig, networkingConfig, nil, vmName)
	if err != nil {
		panic(err)
	}
	log.Println(resp.ID)

	// install additional tools

	return nil
}

func (v *Vmm) StartVM() {
	// call
}

func (v *Vmm) StopVM() {
	// call
}

func (v *Vmm) ListVM() {
	containers, err := v.Client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.HasPrefix(name, VMPrefix) {
				fmt.Printf("%s %s %s\n", container.ID[:10], container.Names, container.Image)
			}
		}
	}
}

func (v *Vmm) removeVM() {
	// call
}

func (v *Vmm) InstallAdeb() {
	// call
}

func (v *Vmm) isCuttlefishImageReady() (bool, error) {

	return true, nil
}

func (v *Vmm) attachDefaultNetwork() {

}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func main() {
	vmm := NewVmm()
	vmm.ListVM()
	vmm.CreateVM()
}
