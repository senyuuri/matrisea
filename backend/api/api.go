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
		v1.DELETE("/vms/:name/", deleteVM)
	}
	router.Run()
}

func listVM(c *gin.Context) {
	vmList, err := v.ListVM()
	if err != nil {
		c.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(200, vmList)
}

func createVM(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func startVM(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func stopVM(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func deleteVM(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}
