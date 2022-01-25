package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"sea.com/matrisea/vmm"
)

var (
	router *gin.Engine
	v      *vmm.VMM
	// websocket - time allowed to read the next pong message from the peer
	pongWait = 10 * time.Second
	// websocket - send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10
)

func main() {
	var err error
	v, err = vmm.NewVMM(getenv("DATA_DIR", "/data"))
	if err != nil {
		log.Fatal(err)
	}
	router = gin.Default()
	config := cors.DefaultConfig()
	config.AllowHeaders = []string{"Origin", "x-requested-with", "content-type"}
	config.AllowOrigins = []string{"http://localhost:3000", "http://192.168.3.112:3000"}
	router.Use(cors.New(config))

	api := router.Group("/api")
	v1 := api.Group("/v1")
	{
		v1.GET("/ws", func(c *gin.Context) {
			createVMHandler(c.Writer, c.Request)
		})
		v1.GET("/vms", listVM)
		v1.POST("/vms/:name/start", startVM)
		v1.POST("/vms/:name/stop", stopVM)
		v1.DELETE("/vms/:name", removeVM)
		v1.GET("/vms/:name/ws", terminalHandler)
		v1.GET("/files/system", getSystemImageList)
		v1.GET("/files/cvd", getCVDImageList)
		v1.POST("/files/upload", uploadFile)
	}
	router.Run()
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO verify origins
		return true
	},
}

type CreateDeviceData struct {
	DeviceName  string `json:"name" binding:"required"`
	DeviceType  string `json:"type" binding:"required"`
	CPU         int    `json:"cpu" binding:"required"`
	RAM         int    `json:"ram" binding:"required"`
	SystemImage string `json:"system_image"`
	CVDImage    string `json:"cvd_image"`
	KernelImage string `json:"kernel_image"`
}

type CreateDeviceRequest struct {
	Type string           `json:"type" binding:"required"`
	Data CreateDeviceData `json:"data"`
}

const (
	STEP_START = iota
	STEP_PREFLIGHT_CHECKS
	STEP_CREATE_VM
	STEP_LOAD_IMAGES
	STEP_START_VM
)

type CreateDeviceResponse struct {
	Step     int    `json:"step" binding:"required"`
	HasError bool   `json:"has_error" binding:"required"`
	ErrorMsg string `json:"error"`
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

func keepAlive(c *websocket.Conn, timeout time.Duration) {
	lastResponse := time.Now()
	c.SetPongHandler(func(msg string) error {
		fmt.Println("Received pong")
		lastResponse = time.Now()
		return nil
	})

	go func() {
		for {
			fmt.Println("Sent ping")
			err := c.WriteMessage(websocket.PingMessage, []byte("keepalive"))
			if err != nil {
				return
			}
			time.Sleep(timeout)
			if time.Since(lastResponse) > pongWait {
				c.Close()
				return
			}
		}
	}()
}

func createVMHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to set websocket upgrade: %+v", err)
		return
	}
	keepAlive(conn, pingPeriod)
	for {
		var req CreateDeviceRequest
		err := conn.ReadJSON(&req)
		if err != nil {
			log.Printf("Failed to parse WS request. Reason: %s\n", err.Error())
			conn.WriteMessage(websocket.TextMessage, []byte("error"+err.Error()))
			break
		}
		log.Printf("ws received: %s\n", req)

		// although we only have 1 type for now. the field is reserved for extensibility
		if req.Type == "create" {
			// 1 - STEP_START: request received
			conn.WriteJSON(&CreateDeviceResponse{
				Step:     STEP_START,
				HasError: false,
			})

			// 2 - STEP_PREFLIGHT_CHECKS
			// check if device exist
			vmList, err := v.VMList()
			if err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_PREFLIGHT_CHECKS,
					HasError: true,
					ErrorMsg: "Failed to retrieve VM info",
				})
				break
			}
			// TODO move name check before submit
			for _, vm := range vmList {
				if vm.Name == req.Data.DeviceName {
					conn.WriteJSON(&CreateDeviceResponse{
						Step:     STEP_PREFLIGHT_CHECKS,
						HasError: true,
						ErrorMsg: "A VM of the same name already exists.",
					})
					break
				}
			}
			// check if image files exist
			systemImagePath := v.UploadDir + "/" + req.Data.SystemImage
			cvdImagePath := v.UploadDir + "/" + req.Data.CVDImage
			images := []string{
				systemImagePath,
				cvdImagePath,
			}
			for _, img := range images {
				if _, err := os.Stat(img); os.IsNotExist(err) {
					conn.WriteJSON(&CreateDeviceResponse{
						Step:     STEP_PREFLIGHT_CHECKS,
						HasError: true,
						ErrorMsg: "Cannot find the selected image(s)",
					})
					break
				}
			}
			// create device folder
			if err := os.Mkdir(v.DevicesDir+"/"+req.Data.DeviceName, 0755); err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_PREFLIGHT_CHECKS,
					HasError: true,
					ErrorMsg: "Failed to create the device folder. Reason: " + err.Error(),
				})
				break
			}
			conn.WriteJSON(&CreateDeviceResponse{
				Step:     STEP_PREFLIGHT_CHECKS,
				HasError: false,
			})

			// 3 - STEP_CREATE_VM
			containerName, err := v.VMCreate(req.Data.DeviceName)
			if err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_CREATE_VM,
					HasError: true,
					ErrorMsg: "Failed to create VM. Reason: " + err.Error(),
				})
				break
			}

			conn.WriteJSON(&CreateDeviceResponse{
				Step:     STEP_CREATE_VM,
				HasError: false,
			})

			// 4 - STEP_LOAD_IMAGES

			// ** Time and space considerations on image loading **
			//
			// Before launching cuttlefish, we need to unzip system images (~13GB) then copy them to /home/vsoc-01 in the container.
			// Since docker API mandatorily tars everything before the copy, if we simply unzip the images and copy each file over,
			// the overhead can be huge (13GB unzip + 13GB tar + 13GB untar).
			//
			// The current solution copies the zip into the container first, then unzip it within the container, so we at least could
			// save lots of time in docker copy (1GB tar + 1GB untar + 13GB unzip).
			//
			// ** Why we dropped OverlayFS support **
			//
			// As of the current implementation, devices that use the same image have to keep duplicated copies. A more idealised
			// solution would be to implement some sort of OverlayFS-like mechanism to achieve image reuse. For that to work, we need a
			// base(lower) read-only directory for images, and a writable layer(upper) for the runtime data. Unfortunatly, the kernel had
			// dropped overlay-on-overlay support due to its hard-to-maintain complexities. And because Docker defaults to overlay2 as its
			// storage driver, asking user to change their global storage driver sorely for Matrisea could affect the compatibility of users'
			// already running workloads. Hence, the OverlayFS idea was dropped.
			//
			// The revoked OverlayFS implementation can be found in commit f77a448e309c3c1f0260d1fec74519c79564e182.
			//

			// Load system image (.zip) and unzip in the container
			err = v.VMLoadFile(containerName, systemImagePath)
			if err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_LOAD_IMAGES,
					HasError: true,
					ErrorMsg: "Failed to load system iamge. Reason: " + err.Error(),
				})
				break
			}
			_, err = v.ContainerExec(containerName, "unzip /home/vsoc-01/"+req.Data.SystemImage, "vsoc-01")
			if err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_LOAD_IMAGES,
					HasError: true,
					ErrorMsg: "Failed to unzip system iamge. Reason: " + err.Error(),
				})
				break
			}
			// Load CVD image (.tar)
			err = v.VMLoadFile(containerName, cvdImagePath)
			if err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_LOAD_IMAGES,
					HasError: true,
					ErrorMsg: "Failed to load system iamge. Reason: " + err.Error(),
				})
				break
			}

			conn.WriteJSON(&CreateDeviceResponse{
				Step:     STEP_LOAD_IMAGES,
				HasError: false,
			})

			// 5 - STEP_START_VM
			_, err = v.VMStart(containerName, "")
			if err != nil {
				conn.WriteJSON(&CreateDeviceResponse{
					Step:     STEP_START_VM,
					HasError: true,
					ErrorMsg: "VM failed to start. Reason: " + err.Error(),
				})
				break
			}
			conn.WriteJSON(&CreateDeviceResponse{
				Step:     STEP_START_VM,
				HasError: false,
			})
			break
		}
		conn.WriteMessage(websocket.TextMessage, []byte("unknown_type"))
	}
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
	// upgrader := websocket.Upgrader{
	// 	CheckOrigin: func(r *http.Request) bool {
	// 		return true
	// 	},
	// }
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
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
		// TODO how to let front end know this error?
		log.Println(err.Error())
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

func getSystemImageList(c *gin.Context) {
	var files []string

	err := filepath.Walk(v.UploadDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".zip") {
			files = append(files, filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{"files": files})
}

func getCVDImageList(c *gin.Context) {
	var files []string

	err := filepath.Walk(v.UploadDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".tar") {
			files = append(files, filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{"files": files})
}

func uploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	// The file cannot be received.
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"message": "No file is received",
		})
		return
	}

	// Retrieve file information
	extension := filepath.Ext(file.Filename)
	log.Println(extension)
	if extension != ".zip" && extension != ".tar" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "Unsupported file formats"},
		)
		return
	}

	// The file is received, so let's save it
	if err := c.SaveUploadedFile(file, v.UploadDir+"/"+file.Filename); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"message": "Unable to save the file",
		})
		return
	}

	// File saved successfully. Return proper result
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
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
