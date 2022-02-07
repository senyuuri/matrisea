package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"sea.com/matrisea/vmm"
)

var (
	router   *gin.Engine
	v        *vmm.VMM
	CFPrefix = "matrisea-cvd-" // container name prefix
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// websocket - time allowed to read the next pong message from the peer
	pongWait = 10 * time.Second
	// websocket - send pings to peer with this period. Must be less than pongWait
	pingPeriod = 9 * time.Second
	// message size limit for websocket
	maxMessageSize int64 = 512
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO verify origins
		return true
	},
}

// VM creation steps, used by wsCreateVM()
type CreateVMStep int

const (
	STEP_START CreateVMStep = iota
	STEP_PREFLIGHT_CHECKS
	STEP_CREATE_VM
	STEP_LOAD_IMAGES
	STEP_START_VM
)

// message type for the main WebSocket connection (/api/v1/ws)
type WsMessageType int

const (
	WS_TYPE_LIST_VM WsMessageType = iota
	WS_TYPE_CREATE_VM
	WS_TYPE_UNKNOWN
)

// Each WsMessageType should define a RequestBody struct and implement AbstractRequestBodyMethod()
type WebSocketRequest struct {
	Type WsMessageType `json:"type" binding:"required"`
	Data RequestBody   `json:"data"`
}

type RequestBody interface {
	// should be an empty method
	AbstractRequestBodyMethod()
}

type WebSocketResponse struct {
	Type     WsMessageType `json:"type" binding:"required"`
	Data     ResponseBody  `json:"data"`
	HasError bool          `json:"has_error" binding:"required"`
	ErrorMsg string        `json:"error"`
}

type ResponseBody interface {
	AbstractResponseBodyMethod()
}

type CreateVMRequest struct {
	DeviceName  string `json:"name" binding:"required"`
	DeviceType  string `json:"type" binding:"required"`
	CPU         int    `json:"cpu" binding:"required"`
	RAM         int    `json:"ram" binding:"required"`
	SystemImage string `json:"system_image"`
	CVDImage    string `json:"cvd_image"`
	KernelImage string `json:"kernel_image"`
}

func (r *CreateVMRequest) AbstractRequestBodyMethod() {}

type CreateVMResponse struct {
	Step CreateVMStep `json:"step" binding:"required"`
}

func (r *CreateVMResponse) AbstractResponseBodyMethod() {}

type ListVMResponse struct {
	VMs []vmm.VMItem `json:"vms" binding:"required"`
}

func (r *ListVMResponse) AbstractResponseBodyMethod() {}

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
		v1.GET("/ws", func(c *gin.Context) { // websocket
			wsHandler(c.Writer, c.Request)
		})
		v1.POST("/vms/:name/start", startVM)
		v1.POST("/vms/:name/stop", stopVM)
		v1.DELETE("/vms/:name", removeVM)
		v1.GET("/vms/:name/ws", terminalHandler) // websocket
		v1.GET("/files/system", getSystemImageList)
		v1.GET("/files/cvd", getCVDImageList)
		v1.POST("/files/upload", uploadFile)
	}
	router.Run()
}

// Open a shared WS connection for features that require either
// - periodic query e.g. wsListVM() OR
// - live update e.g. wsCreateVM()
//
// As gorilla/websocket doesn't support concurrent read/write, we must start two
// go routines that strictly isolate read/write from each other
// See example https://github.com/gorilla/websocket/blob/master/examples/chat/client.go
//
// How to implementing a new WS message type xxx
// - Define a new type in WS_TYPE_xxx in WsMessageType
// - (Optional) define a new struct xxxRequest and implement AbstractRequestBodyMethod()
// - (Optional) define a new struct xxxResponse and implement AbstractResponseBodyMethod()
// - create a handler with name starts with `ws` e.g. wsXxx
// - register the handler in wsHandler() as a switch case
func wsHandler(w http.ResponseWriter, r *http.Request) {
	wsConn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to set websocket upgrade: %+v", err)
		return
	}
	conn := &Connection{
		conn: wsConn,
		send: make(chan interface{}),
	}
	conn.SetMessageHandler(processWSMessage)

	go conn.readPump()
	go conn.writePump()
}

func processWSMessage(c *Connection, buf []byte) {
	var objmap map[string]json.RawMessage
	err := json.Unmarshal(buf, &objmap)
	if err != nil {
		log.Println(err.Error())
	}
	var reqType WsMessageType
	err = json.Unmarshal(objmap["type"], &reqType)
	if err != nil {
		log.Println(err.Error())
	}

	switch reqType {
	case WS_TYPE_LIST_VM:
		// log.Printf("/api/v1/ws invoke wsListVM()") // comment out since it's too chatty
		wsListVM(c)

	case WS_TYPE_CREATE_VM:
		log.Printf("/api/v1/ws invoke wsCreateVM()")
		var req CreateVMRequest
		err = json.Unmarshal(objmap["data"], &req)
		if err != nil {
			log.Println(err.Error())
		}
		wsCreateVM(c, req)

	default:
		c.send <- &WebSocketResponse{
			Type:     WS_TYPE_UNKNOWN,
			HasError: true,
			ErrorMsg: fmt.Sprintf("Unknown websocket message type %d", reqType),
		}
	}
}

// Get a list of existing VMs as long as there's a container for it, regardless of the container status
// TODO get crosvm process status in running containers
func wsListVM(c *Connection) {
	vmList, err := v.VMList()
	if err != nil {
		c.send <- &WebSocketResponse{
			Type:     WS_TYPE_LIST_VM,
			HasError: true,
			ErrorMsg: "Failed to retrieve VM info",
		}
	}
	c.send <- &WebSocketResponse{
		Type: WS_TYPE_LIST_VM,
		Data: &ListVMResponse{
			VMs: vmList,
		},
	}
}

// Create and start a new VM in multiple steps (CreateVMStep).
// Send live updates through websocket
func wsCreateVM(c *Connection, req CreateVMRequest) {
	// 1 - STEP_START: request received
	wsCreateVMCompleteStep(c, STEP_START)

	// 2 - STEP_PREFLIGHT_CHECKS
	vmList, err := v.VMList()
	if err != nil {
		wsCreateVMFailStep(c, STEP_PREFLIGHT_CHECKS, "Failed to retrieve VM info")
		return
	}
	// check if a device of the same name already exists
	// TODO move name check before submit
	for _, vm := range vmList {
		if vm.Name == req.DeviceName {
			wsCreateVMFailStep(c, STEP_PREFLIGHT_CHECKS, "A VM of the same name already exists.")
			return
		}
	}
	// check if image files exist
	systemImagePath := v.UploadDir + "/" + req.SystemImage
	cvdImagePath := v.UploadDir + "/" + req.CVDImage
	images := []string{
		systemImagePath,
		cvdImagePath,
	}
	for _, img := range images {
		if _, err := os.Stat(img); os.IsNotExist(err) {
			wsCreateVMFailStep(c, STEP_PREFLIGHT_CHECKS, "Cannot find the selected image(s)")
			return
		}
	}
	// create device folder
	folderName := CFPrefix + req.DeviceName
	if err := os.Mkdir(path.Join(v.DevicesDir+"/"+folderName), 0755); err != nil {
		wsCreateVMFailStep(c, STEP_PREFLIGHT_CHECKS, "Failed to create the device folder. Reason: "+err.Error())
		return
	}
	wsCreateVMCompleteStep(c, STEP_PREFLIGHT_CHECKS)

	// 3 - STEP_CREATE_VM
	match, _ := regexp.MatchString("^[a-zA-z0-9-_]+$", req.DeviceName)
	if !match {
		wsCreateVMFailStep(c, STEP_CREATE_VM, "Failed to create VM. Reason: device name contains characters other than alphanumerics and _-")
		return
	}
	if len(req.DeviceName) > 20 {
		wsCreateVMFailStep(c, STEP_CREATE_VM, "Failed to create VM. Reason: device name exceed 20 characters")
		return
	}
	containerName, err := v.VMCreate(req.DeviceName)

	if err != nil {
		wsCreateVMFailStep(c, STEP_CREATE_VM, "Failed to create VM. Reason: "+err.Error())
		return
	}
	wsCreateVMCompleteStep(c, STEP_CREATE_VM)

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
		wsCreateVMFailStep(c, STEP_LOAD_IMAGES, "Failed to load system iamge. Reason: "+err.Error())
		return
	}
	err = v.VMUnzipImage(containerName, req.SystemImage)
	if err != nil {
		wsCreateVMFailStep(c, STEP_LOAD_IMAGES, "Failed to unzip system iamge. Reason: "+err.Error())
		return
	}
	// Load CVD image (.tar)
	err = v.VMLoadFile(containerName, cvdImagePath)
	if err != nil {
		wsCreateVMFailStep(c, STEP_LOAD_IMAGES, "Failed to load system iamge. Reason: "+err.Error())
		return
	}
	wsCreateVMCompleteStep(c, STEP_LOAD_IMAGES)

	// 5 - STEP_START_VM
	_, err = v.VMStart(containerName, "")
	if err != nil {
		wsCreateVMFailStep(c, STEP_START_VM, "VM failed to start. Reason: "+err.Error())
		return
	}
	wsCreateVMCompleteStep(c, STEP_START_VM)
}

func wsCreateVMCompleteStep(c *Connection, step CreateVMStep) {
	log.Printf("CreateVM done step %d", step)
	c.send <- &WebSocketResponse{
		Type: WS_TYPE_CREATE_VM,
		Data: &CreateVMResponse{
			Step: step,
		},
	}
}

func wsCreateVMFailStep(c *Connection, step CreateVMStep, errorMsg string) {
	log.Printf("CreateVM failed at step %d due to %s", step, errorMsg)
	c.send <- &WebSocketResponse{
		Type: WS_TYPE_CREATE_VM,
		Data: &CreateVMResponse{
			Step: step,
		},
		HasError: true,
		ErrorMsg: errorMsg,
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
	go wsWriterCopy(conn, hijackedResp.Conn)
	go wsReaderCopy(conn, hijackedResp.Conn)
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
func wsWriterCopy(writer *websocket.Conn, reader io.Reader) {
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
