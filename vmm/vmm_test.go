package vmm

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// Pre-test setup which also invokes NewVMM, VMCreate
func setup() {
	rand.Seed(time.Now().UnixNano())

	testBatch := "matrisea-test-" + randSeq(6) + "-"
	var err error
	dataDir, err = ioutil.TempDir("", testBatch)
	if err != nil {
		log.Fatal(err)
	}

	v = NewMockVMM(dataDir, testBatch)
	containerName, err = v.VMCreate("01", 2, 4, "Android 12", "")
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
	v.Close()
	os.RemoveAll(dataDir)

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

func TestGetContainerIDByInvalidName(t *testing.T) {
	_, err := v.getContainerIDByName("invalid-name")
	assert.Error(t, err)
}

func TestGetContainerIDByName(t *testing.T) {
	containerID, err := v.getContainerIDByName(containerName)
	assert.Nil(t, err)

	cmd := fmt.Sprintf("docker ps | grep %s | grep -q %s", containerName, containerID[:12])
	err = exec.Command("bash", "-c", cmd).Run()
	assert.Nil(t, err)
}

func TestVMLoadNonExistFileToContainer(t *testing.T) {
	err := v.VMLoadFile(containerName, dataDir+"/testfile-non-exist")
	assert.Error(t, err)
}

func TestCopyTarToContainer(t *testing.T) {
	cmd := exec.Command("sh", "-c", "touch testfile && tar -cvf test.tar testfile")
	cmd.Dir = dataDir
	assert.Nil(t, cmd.Run())

	err := v.containerCopyFile(dataDir+"/test.tar", containerName, "/home/vsoc-01")
	assert.Nil(t, err)

	cmd = exec.Command("docker", "exec", containerName, "ls", "/home/vsoc-01/testfile")
	assert.Nil(t, cmd.Run())
}

func TestCopyNonTarToContainer(t *testing.T) {
	cmd := exec.Command("sh", "-c", "touch testfile")
	cmd.Dir = dataDir
	assert.Nil(t, cmd.Run())

	err := v.containerCopyFile(dataDir+"/testfile", containerName, "/home/vsoc-01")
	assert.Nil(t, err)

	cmd = exec.Command("docker", "exec", containerName, "ls", "/home/vsoc-01/testfile")
	assert.Nil(t, cmd.Run())
}

func TestContainerExec(t *testing.T) {
	// non exist user
	resp, err := v.containerExec(containerName, "unameee", "unknown-user")
	assert.Nil(t, err)
	assert.NotZero(t, resp.ExitCode)

	// non exist executable
	resp, err = v.containerExec(containerName, "unameee", "vsoc-01")
	assert.Nil(t, err)
	assert.NotZero(t, resp.ExitCode)
	assert.Equal(t, "/bin/sh: 1: unameee: not found\n", resp.errBuffer.String())

	// non-zero exit code
	resp, err = v.containerExec(containerName, "uname -z", "vsoc-01")
	assert.Nil(t, err)
	assert.NotZero(t, resp.ExitCode)
	assert.True(t, strings.HasPrefix(resp.errBuffer.String(), "uname: invalid option"))

	// good cmd
	resp, err = v.containerExec(containerName, "uname", "vsoc-01")
	assert.Nil(t, err)
	assert.Zero(t, resp.ExitCode)
	assert.Equal(t, "Linux\n", resp.outBuffer.String())
}

func TestListCuttlefishContainers(t *testing.T) {
	cflist, err := v.listCuttlefishContainers()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(cflist))
}

func TestIsValidManagedContainer(t *testing.T) {
	err := v.VMLoadFile("matrisea-non-exist", dataDir+"/testfile-0")
	assert.Error(t, err)
}

func TestContainerListFilesNonExistFolder(t *testing.T) {
	_, err := v.ContainerListFiles(containerName, "/tmp/non-exist-folder")
	assert.Error(t, err)
}

func TestContainerAttachToProcessThenKill(t *testing.T) {
	_, hijackedResp, err := v.ContainerAttachToProcess(containerName, []string{"top"}, []string{})
	assert.Nil(t, err)
	defer func() {
		hijackedResp.Conn.Write([]byte("exit\r"))
		hijackedResp.Close()
	}()
	// top process should keep running
	cid, err := v.getContainerIDByName(containerName)
	assert.Nil(t, err)
	cmd := exec.Command("bash", "-c", fmt.Sprintf("docker exec %s ps aux | grep -q [t]op", cid))
	assert.Nil(t, cmd.Run())

	err = v.ContainerKillProcess(containerName, "top")
	assert.Nil(t, err)
	// top should has been killed. grep -q returns a non-zero value if no match found
	cmd = exec.Command("bash", "-c", fmt.Sprintf("docker exec %s ps aux | grep -q [t]op", cid))
	assert.Error(t, cmd.Run())
}

func TestVMList(t *testing.T) {
	cfList, err := v.VMList()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(cfList))
}

// Test the full cycle from downloading aosp-main images from Android CI to start/stop the VM.
// Use require instead of assert to fail fast.
// Note: this could take 2-5 minutes depends on network conditions
func TestVMMIntegration(t *testing.T) {
	err := v.VMPreBootSetup(containerName)
	require.Nil(t, err)

	cid, err := v.getContainerIDByName(containerName)
	require.Nil(t, err)
	cmd := exec.Command("bash", "-c", fmt.Sprintf("docker exec %s ps aux | grep -q [w]ebsockify", cid))
	require.Nil(t, cmd.Run())

	clist, err := v.listCuttlefishContainers()
	require.Nil(t, err)
	var container types.Container
	for _, c := range clist {
		if cid == c.ID {
			container = c
		}
	}
	require.NotNil(t, container)
	status, _ := v.getVMStatus(container)
	require.Equal(t, VMReady, status)

	// Download the latest system and cvd images
	log.Println("Start download-aosp.sh")
	cmd = exec.Command("cp", "download-aosp.sh", v.UploadDir)
	require.Nil(t, cmd.Run())
	cmd = exec.Command("bash", "-c", "set -x && ./download-aosp.sh -A -C -D -K -X -a $(uname -m)")
	cmd.Dir = v.UploadDir
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		m := scanner.Text()
		fmt.Print(m + " ")
	}
	require.Nil(t, cmd.Wait())

	// Get the filename of the system image
	systemImage := ""
	err = filepath.Walk(v.UploadDir, func(path string, info os.FileInfo, err error) error {
		require.Nil(t, err)
		fileName := filepath.Base(path)
		if strings.HasPrefix(fileName, "aosp_cf_x86_64_phone-img-") {
			systemImage = fileName
		}
		return nil
	})
	require.Nil(t, err)

	// Load system and CVD images
	err = v.VMLoadFile(containerName, path.Join(v.UploadDir, systemImage))
	require.Nil(t, err)
	err = v.VMUnzipImage(containerName, systemImage)
	require.Nil(t, err)
	err = v.VMLoadFile(containerName, path.Join(v.UploadDir, "cvd-host_package.tar.gz"))
	require.Nil(t, err)

	// Try start and stop the VM
	err = v.VMStart(containerName, false, "", func(lines string) {
		fmt.Println(lines)
	})
	require.Nil(t, err)

	status, _ = v.getVMStatus(container)
	require.Equal(t, VMRunning, status)

	err = v.VMStop(containerName)
	require.Nil(t, err)
}
