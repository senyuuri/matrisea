package vmm

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var vmm *VMM
var vmName string

// CreateVM/RemoveVM are tested in setup()/shutdown()
func TestMain(m *testing.M) {
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

	// TODO: for local debugging only. to be removed
	vmm.PruneVMs()

	// create a common VM for other tests
	vmName, err = vmm.CreateVM("android11-gsi-cf")
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
		t.Error(err.Error())
	}

	cmd := fmt.Sprintf("docker ps | grep %s | grep -q %s", vmName, containerID[:12])
	err = exec.Command("bash", "-c", cmd).Run()
	if err != nil {
		t.Error(err.Error())
	}
}

func TestGetContainerNameByID(t *testing.T) {
	cmd := fmt.Sprintf("docker ps | grep %s | cut -d ' ' -f 1 | tr -d $'\n'", vmName)
	resp, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		t.Error(err.Error())
	}
	containerID := strings.TrimRight(string(resp), "\n")
	name, err := vmm.getContainerNameByID(containerID)
	if err != nil || name != vmName {
		t.Error(err.Error())
	}
}

func TestCopyToContainer(t *testing.T) {
	dir, err := ioutil.TempDir("", "matrisea-test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cmd := exec.Command("sh", "-c", "touch testfile && tar -cvf test.tar testfile")
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		t.Error(err.Error())
	}

	err = vmm.CopyToContainer(dir+"/test.tar", vmName, "/home/vsoc-01")
	if err != nil {
		t.Error(err.Error())
	}
	cmd = exec.Command("docker", "exec", vmName, "ls", "/home/vsoc-01/testfile")
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() != 0 {
				t.Error(err.Error())
			}
		}
	}
}

func TestStartStopVM(t *testing.T) {
	_, err := vmm.StartVM(vmName, "")
	if err != nil {
		t.Error(err.Error())
	}

	// time.Sleep(30 * time.Second)
	// check if cvd log file is created in the container

	// wait until VM is ready

	// vmm.StopVM(vmName)
}

func TestVMMIntegration(t *testing.T) {

}
