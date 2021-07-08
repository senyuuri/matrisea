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
	name, _ := vmm.getContainerNameByID(id)

	vmm.loadImages(name)
	vmm.StartVM(name, "")
}
