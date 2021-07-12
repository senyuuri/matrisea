package vmm

import (
	"testing"
)

func TestVMMIntegration(t *testing.T) {
	vmm := NewVMM(
		"/data/workspace/matrisea/images/",
		"/data/workspace/matrisea/upload/",
	)

	// vmm.PruneVMs()
	// name, _ := vmm.CreateVM()
	// vmm.PrintVMs()

	// vmm.LoadImages(
	// 	name,
	// 	"/data/workspace/matrisea/upload/aosp_cf_x86_64_phone-img-7530437.zip",
	// 	"/data/workspace/matrisea/upload/cvd-host_package.tar",
	// )

	vms, _ := vmm.ListVM()
	vmm.StartVM(getCFContainerName(vms[0]), "")
	// time.Sleep(30 * time.Second)
	// vmm.StopVM(getCFContainerName(vms[0]))
}
