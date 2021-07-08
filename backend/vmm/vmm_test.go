package vmm

import (
	"fmt"
	"testing"
)

func TestVMMIntegration(t *testing.T) {
	fmt.Printf("aaaa")
	vmm := NewVMM()
	vmm.PruneVMs()
	id, _ := vmm.CreateVM()
	vmm.PrintVMs()
	fmt.Println(vmm.getContainerNameByID(id))

	// vmList, _ := vmm.ListVM()
	// vmm.loadImages(getCFContainerName(vmList[0]))
	// vmm.StartVM(getCFContainerName(vmList[0]), "")
}
