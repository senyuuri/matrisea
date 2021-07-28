package vmm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var vmm *VMM
var vmName string

// CreateVM/RemoveVM are tested in setup()/shutdown()
func TestMain(m *testing.M) {
	// vmm.PruneVMs()
	setup()
	retCode := m.Run()
	// shutdown()
	os.Exit(retCode)
}

func setup() {
	var err error
	vmm, err = NewVMM(
		"/home/senyuuri/matrisea/data/",
	)
	if err != nil {
		fmt.Printf("Failed to create a docker API client. Reason: %s\n", err.Error())
		os.Exit(1)
	}

	// create a common VM for other tests
	vmName, err = vmm.CreateVM()
	if err != nil {
		fmt.Printf("Failed to CreateVM(). Reason: %s\n", err.Error())
		os.Exit(1)
	}

	// use docker CLI to double check if the container is running
	cmd := fmt.Sprintf("docker ps | grep %s | grep -q Up", vmName)
	err = exec.Command("bash", "-c", cmd).Run()
	if err != nil {
		fmt.Printf("Failed to CreateVM() because the container isn't created or isn't running.\n")
		os.Exit(1)
	}
}

func shutdown() {
	err := vmm.RemoveVM(vmName)
	if err != nil {
		fmt.Printf("Failed to RemoveVM(). Reason: %s\n", err.Error())
		os.Exit(1)
	}

	cmd := fmt.Sprintf("docker ps | grep -q %s", vmName)
	err = exec.Command("bash", "-c", cmd).Run()
	// nil err means grep returns zero, i.e. the container is still running
	if err == nil {
		fmt.Printf("DeleteVM() succeeded but the container is still running.\n")
		os.Exit(1)
	}
}

func TestGetContainerIDByName(t *testing.T) {
	containerID, err := vmm.getContainerIDByName(vmName)
	if err != nil {
		t.Log(err.Error())
		t.Fail()
	}

	cmd := fmt.Sprintf("docker ps | grep %s | grep -q %s", vmName, containerID[:12])
	err = exec.Command("bash", "-c", cmd).Run()
	if err != nil {
		t.Fail()
	}
}

func TestGetContainerNameByID(t *testing.T) {
	cmd := fmt.Sprintf("docker ps | grep %s | cut -d ' ' -f 1 | tr -d $'\n'", vmName)
	resp, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		t.Fail()
	}
	containerID := strings.TrimRight(string(resp), "\n")
	name, err := vmm.getContainerNameByID(containerID)
	if err != nil || name != vmName {
		t.Fail()
	}
}

func TestStartStopVM(t *testing.T) {
	err := vmm.StartVM(vmName, "")
	if err != nil {
		t.Log(err.Error())
		t.Fail()
	}

	// time.Sleep(30 * time.Second)
	// check if cvd log file is created in the container

	// wait until VM is ready

	// vmm.StopVM(vmName)
}

func TestVMMIntegration(t *testing.T) {

}
