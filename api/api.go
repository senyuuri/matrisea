package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
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
	var err error
	v, err = vmm.NewVMM(getenv("IMAGE_DIR", "/data/workspace/matrisea/images/"))
	if err != nil {
		log.Fatal(err)
	}
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
	vmList, err := v.VMList()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, vmList)
}

func createVM(c *gin.Context) {
	// create and run a container
	name, err := v.VMCreate("android11-gsi-cf")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// unzip/untar selected images to the container's image folder on the host
	// if err := v.LoadImages(name, aosp_file, cvd_file); err != nil {
	// 	c.JSON(500, gin.H{"error": err.Error()})
	// 	return
	// }
	c.JSON(200, gin.H{"container_name": name})
}

func startVM(c *gin.Context) {
	name := c.Param("name")
	// TODO add default options
	if _, err := v.VMStart(name, ""); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func stopVM(c *gin.Context) {
	name := c.Param("name")
	if err := v.VMStop(name); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func removeVM(c *gin.Context) {
	name := c.Param("name")
	if err := v.VMRemove(name); err != nil {
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

// TODO
// - get VM name from the request
// - get VM internal IP
// - assign a port from range 10000-10100
// - start a wsproxy
// - set a timeout timer and shutdown the port if inactive for x min
// - authentication?
//
// https://stackoverflow.com/questions/39320025/how-to-stop-http-listenandserve/42533360
func startVNCTunnel() {
	u, err := url.Parse("ws://172.17.0.2:6080")
	if err != nil {
		log.Fatalln(err)
	}

	err = http.ListenAndServe(":10000", NewProxy(u))
	if err != nil {
		log.Fatalln(err)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
