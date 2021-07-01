package vmm

import (
	"testing"
)

func TestVMMIntegration(t *testing.T) {
	vmm := NewVmm()
	vmm.pruneVMs()
	vmm.CreateVM()
	vmList := vmm.ListVM()
	vmm.StartVM(getCFContainerName(vmList[0]), "")
}
