package vmm

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

var v *VMM
var containerName string
var dataDir string

// Create a cuttlefish container and load relevant images before other tests.
// Destroy the container on cleanup().
func TestMain(m *testing.M) {
	setup()
	retCode := m.Run()
	cleanup()
	os.Exit(retCode)
}

// Pre-test setup which also invoke NewVMM and VMCreate
func setup() {
	testBatch := "matrisea-test-" + randSeq(6) + "-"
	var err error
	dataDir, err = ioutil.TempDir("", testBatch)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	v = NewMockVMM(dataDir, testBatch)
	containerName, err = v.VMCreate("01", 2, 4, "Android 12")
	if err != nil {
		log.Printf("VMCreate failed. reason: %v\n", err)
		if err := v.VMRemove(containerName); err != nil {
			log.Fatalf("VMRemove failed to clean up the test container. reason: %v\n", err)
		}
		log.Fatalf("Successfully deleted test container %s\n", containerName)
	}
}

// Post-test clean up which also tests VMRemove
func cleanup() {
	err := v.VMRemove(containerName)
	if err != nil {
		log.Fatalf("RemoveVM failed. reason: %v\n", err)
	}

	cmd := fmt.Sprintf("docker ps | grep -q %s", containerName)
	err = exec.Command("bash", "-c", cmd).Run()
	// nil err means grep returns zero, i.e. the container is still running
	if err == nil {
		log.Fatalf("DeleteVM() succeeded but the container is still running. reason:%v\n", err)
	}
	log.Printf("Successfully deleted test container %s\n", containerName)
}

func TestNewVMM(t *testing.T) {
	assert.DirExists(t, path.Join(dataDir, "devices"))
	assert.DirExists(t, path.Join(dataDir, "db"))
	assert.DirExists(t, path.Join(dataDir, "upload"))
}

// func TestGetContainerIDByName(t *testing.T) {
// 	containerID, err := vmm.getContainerIDByName(vmName)
// 	if err != nil {
// 		t.Error(err.Error())
// 	}

// 	cmd := fmt.Sprintf("docker ps | grep %s | grep -q %s", vmName, containerID[:12])
// 	err = exec.Command("bash", "-c", cmd).Run()
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// }

// func TestGetContainerNameByID(t *testing.T) {
// 	cmd := fmt.Sprintf("docker ps | grep %s | cut -d ' ' -f 1 | tr -d $'\n'", vmName)
// 	resp, err := exec.Command("bash", "-c", cmd).Output()
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// 	containerID := strings.TrimRight(string(resp), "\n")
// 	name, err := vmm.getContainerNameByID(containerID)
// 	if err != nil || name != vmName {
// 		t.Error(err.Error())
// 	}
// }

// func TestCopyTarToContainer(t *testing.T) {

// 	cmd := exec.Command("sh", "-c", "touch testfile && tar -cvf test.tar testfile")
// 	cmd.Dir = dir
// 	err = cmd.Run()
// 	if err != nil {
// 		t.Error(err.Error())
// 	}

// 	err = vmm.containerCopyFile(dir+"/test.tar", vmName, "/home/vsoc-01")
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// 	cmd = exec.Command("docker", "exec", vmName, "ls", "/home/vsoc-01/testfile")
// 	if err := cmd.Run(); err != nil {
// 		if exitError, ok := err.(*exec.ExitError); ok {
// 			if exitError.ExitCode() != 0 {
// 				t.Error(err.Error())
// 			}
// 		}
// 	}
// }

// func TestCopyNonTarToContainer(t *testing.T) {
// 	dir, err := ioutil.TempDir("", "matrisea-test")
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	defer os.RemoveAll(dir)

// 	cmd := exec.Command("sh", "-c", "touch testfile")
// 	cmd.Dir = dir
// 	err = cmd.Run()
// 	if err != nil {
// 		t.Error(err.Error())
// 	}

// 	err = vmm.containerCopyFile(dir+"/testfile", vmName, "/home/vsoc-01")
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// 	cmd = exec.Command("docker", "exec", vmName, "ls", "/home/vsoc-01/testfile")
// 	if err := cmd.Run(); err != nil {
// 		if exitError, ok := err.(*exec.ExitError); ok {
// 			if exitError.ExitCode() != 0 {
// 				t.Error(err.Error())
// 			}
// 		}
// 	}
// }

// func TestStartVNCProxy(t *testing.T) {
// 	if err := vmm.startVNCProxy(vmName); err != nil {
// 		t.Fatalf(err.Error())
// 	}

// 	cid, err := vmm.getContainerIDByName(vmName)
// 	if err != nil {
// 		t.Fatalf(err.Error())
// 	}
// 	cmd := fmt.Sprintf("docker exec %s ps aux | grep -q websockify", cid)
// 	err = exec.Command("bash", "-c", cmd).Run()
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// }

// func TestContainerExec(t *testing.T) {
// 	resp, err := vmm.ContainerExec(vmName, "uname -a", "vsoc-01")
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// 	log.Println(resp.ExitCode)
// 	log.Println(resp.outBuffer.String())
// 	log.Println(resp.errBuffer.String())
// 	if resp.ExitCode != 0 || !strings.Contains(resp.outBuffer.String(), "Linux") {
// 		t.Error()
// 	}
// }

// // test the full cycle from loading device images to start/stop VM
// // assumes images have already been downloaded into the `images` folder
// //
// // TODO setup CI to pull latest images from ci.android.com
// func TestVMMIntegration(t *testing.T) {
// 	status, err := vmm.getVMStatus(vmName)
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// 	if status != VMReady {
// 		t.Fatalf("Incorrect VM status. Status: %d\n", status)
// 	}

// 	err = filepath.Walk(
// 		"/home/senyuuri/matrisea/data/images/android11-gsi-cf",
// 		func(path string, f os.FileInfo, err error) error {
// 			if !f.IsDir() {
// 				cperr := vmm.containerCopyFile(path, vmName, "/home/vsoc-01")
// 				if cperr != nil {
// 					t.Fatal(cperr.Error())
// 				}

// 				if strings.HasSuffix(path, ".zip") {
// 					_, srcFile := filepath.Split(path)
// 					resp, err := vmm.ContainerExec(vmName, "unzip "+srcFile+" -d /home/vsoc-01/", "vsoc-01")
// 					if err != nil {
// 						t.Fatal(cperr.Error())
// 					}
// 					if resp.ExitCode != 0 {
// 						t.Fatal()
// 					}
// 				}
// 			}
// 			return nil
// 		},
// 	)

// 	if err != nil {
// 		t.Fatal(err.Error())
// 	}

// 	_, err = vmm.VMStart(vmName, "")
// 	if err != nil {
// 		t.Fatal(err.Error())
// 	}

// 	status, err = vmm.getVMStatus(vmName)
// 	if err != nil {
// 		t.Error(err.Error())
// 	}
// 	if status != VMRunning {
// 		t.Fatalf("Incorrect VM status. Status: %d\n", status)
// 	}

// 	// time.Sleep(30 * time.Second)
// 	// check if cvd log file is created in the container

// 	// wait until VM is ready

// 	// vmm.StopVM(vmName)
// }
