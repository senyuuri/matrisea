package main

import (
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"sea.com/matrisea/vmm"
)

var (
	router *gin.Engine
	v      *vmm.VMM
)

func main() {
	v = vmm.NewVMM(
		getenv("IMAGE_DIR", "/data/workspace/matrisea/images/"),
		getenv("UPLOAD_DIR", "/data/workspace/matrisea/upload/"),
	)
	router = gin.Default()
	api := router.Group("/api")

	v1 := api.Group("/v1")
	{
		v1.GET("/vms/", listVM)
		v1.POST("/vms/", createVM)
		v1.POST("/vms/:name/start", startVM)
		v1.POST("/vms/:name/stop", stopVM)
		v1.DELETE("/vms/:name/", removeVM)
		v1.GET("/vms/:name/ws", terminalHandler)
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
	aosp_file := c.PostForm("aosp_file")
	cvd_file := c.PostForm("cvd_file")
	// create and run a container
	name, err := v.CreateVM()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// unzip/untar selected images to the container's image folder on the host
	if err := v.LoadImages(name, aosp_file, cvd_file); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"container_name": name})
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

func terminalHandler(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer conn.Close()

	// read container name from URL params
	container := c.Param("name")

	// run bash in container and get the hijacked session
	hijackedResp, err := v.AttachToTerminal(container)
	if err != nil {
		log.Fatalf(err.Error())
		return
	}

	// clean up after quit
	defer hijackedResp.Close()
	defer func() {
		hijackedResp.Conn.Write([]byte("exit\r"))
	}()

	// forward read/write to websocket
	go func() {
		wsWriterCopy(hijackedResp.Conn, conn)
	}()
	wsReaderCopy(conn, hijackedResp.Conn)
}

// write terminal output to front end
func wsWriterCopy(reader io.Reader, writer *websocket.Conn) {
	buf := make([]byte, 8192)
	for {
		nr, err := reader.Read(buf)
		if nr > 0 {
			err := writer.WriteMessage(websocket.BinaryMessage, buf[0:nr])
			if err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// send front end input to terminal
func wsReaderCopy(reader *websocket.Conn, writer io.Writer) {
	for {
		messageType, p, err := reader.ReadMessage()
		if err != nil {
			return
		}
		if messageType == websocket.TextMessage {
			writer.Write(p)
		}
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
