package main

import (
	"github.com/gin-gonic/gin"
	"sea.com/matrisea/vmm"
)

var router *gin.Engine
var v *vmm.VMM

func main() {
	v = vmm.NewVMM()
	router = gin.Default()
	api := router.Group("/api")

	v1 := api.Group("/v1")
	{
		v1.GET("/vms/", listVM)
		v1.POST("/vms/", createVM)
		v1.POST("/vms/:name/start", startVM)
		v1.POST("/vms/:name/stop", stopVM)
		v1.DELETE("/vms/:name/", removeVM)
	}
	router.Run()
}

// TODO get crosvm process status in running containers
func listVM(c *gin.Context) {
	vmList, err := v.ListVM()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, vmList)
}

func createVM(c *gin.Context) {
	containerID, err := v.CreateVM()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"container_id": containerID})
}

func startVM(c *gin.Context) {
	name := c.Param("name")
	// TODO add default options
	if err := v.StartVM(name, ""); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func stopVM(c *gin.Context) {
	name := c.Param("name")
	if err := v.StopVM(name); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func removeVM(c *gin.Context) {
	name := c.Param("name")
	if err := v.RemoveVM(name); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}
