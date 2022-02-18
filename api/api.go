package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"sea.com/matrisea/vmm"
)

var (
	router   *gin.Engine
	v        *vmm.VMM
	CFPrefix = "matrisea-cvd-" // container name prefix
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
	WS_TYPE_INSTALL_APK
	WS_TYPE_CREATE_VM_LOG
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
	AOSPVersion string `json:"aosp_version"`
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

type InstallAPKRequest struct {
	DeviceName string `json:"name" binding:"required"`
	File       string `json:"file" binding:"required"`
}

func (r *InstallAPKRequest) AbstractRequestBodyMethod() {}

type InstallAPKResponse struct {
	DeviceName string `json:"name" binding:"required"`
	File       string `json:"file" binding:"required"`
}

func (r *InstallAPKResponse) AbstractResponseBodyMethod() {}

type CreateVMLogResponse struct {
	Log string `json:"log"`
}

func (r *CreateVMLogResponse) AbstractResponseBodyMethod() {}

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
		v1.GET("/vms/:name", getVM)
		v1.POST("/vms/:name/start", startVM)
		v1.POST("/vms/:name/stop", stopVM)
		v1.POST("/vms/:name/upload", uploadDeviceFile)
		v1.GET("/vms/:name/apks", getApkFileList)
		v1.DELETE("/vms/:name", removeVM)
		v1.GET("/vms/:name/ws", TerminalHandler)           // websocket
		v1.GET("/vms/:name/log/:source", LogStreamHandler) // websocket
		v1.GET("/files/system", getSystemImageList)
		v1.GET("/files/cvd", getCVDImageList)
		v1.POST("/files/upload", uploadImageFile)
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
	conn.SetMessageHandler(wsMainPageHandler)

	go conn.readPump()
	go conn.writePump()
}

func wsMainPageHandler(c *Connection, buf []byte) {
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
		var createReq CreateVMRequest
		err = json.Unmarshal(objmap["data"], &createReq)
		if err != nil {
			wsError(c, WS_TYPE_CREATE_VM, "Invalid message type")
		}
		wsCreateVM(c, createReq)

	case WS_TYPE_INSTALL_APK:
		log.Printf("/api/v1/ws invoke wsInstallAPK()")
		var installReq InstallAPKRequest
		err = json.Unmarshal(objmap["data"], &installReq)
		if err != nil {
			log.Println(err.Error())
		}
		wsInstallAPK(c, installReq)

	default:
		wsError(c, WS_TYPE_UNKNOWN, fmt.Sprintf("Unknown websocket message type %d", reqType))
	}
}

func wsError(c *Connection, t WsMessageType, msg string) {
	log.Printf("wsError: %s, %d, %s", c.conn.RemoteAddr(), t, msg)
	c.send <- &WebSocketResponse{
		Type:     t,
		HasError: true,
		ErrorMsg: msg,
	}
}

func wsInstallAPK(c *Connection, req InstallAPKRequest) {
	containerName := CFPrefix + req.DeviceName
	err := v.InstallAPK(containerName, req.File)
	if err != nil {
		wsError(c, WS_TYPE_INSTALL_APK, err.Error())
		return
	}
	c.send <- &WebSocketResponse{
		Type:     WS_TYPE_INSTALL_APK,
		HasError: false,
		Data: &InstallAPKResponse{
			DeviceName: req.DeviceName,
			File:       req.File,
		},
	}
}

// Get a list of existing VMs as long as there's a container for it, regardless of the container status
// TODO get crosvm process status in running containers
func wsListVM(c *Connection) {
	vmList, err := v.VMList()
	if err != nil {
		log.Println("Error: VMList failed due to ", err.Error())
		c.send <- &WebSocketResponse{
			Type:     WS_TYPE_LIST_VM,
			HasError: true,
			ErrorMsg: "Failed to retrieve VM info due to " + err.Error(),
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
	containerName, err := v.VMCreate(req.DeviceName, req.CPU, req.RAM, req.AOSPVersion)

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
	err = v.VMStart(containerName, false, "", func(out string) {
		c.send <- &WebSocketResponse{
			Type: WS_TYPE_CREATE_VM_LOG,
			Data: &CreateVMLogResponse{
				Log: out,
			},
		}
	})
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

func getVM(c *gin.Context) {
	name := c.Param("name")
	vmList, err := v.VMList()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	for _, vm := range vmList {
		if vm.Name == name {
			c.JSON(200, vm)
			return
		}
	}
	c.JSON(500, gin.H{"error": "VM not found"})
}

func startVM(c *gin.Context) {
	name := CFPrefix + c.Param("name")
	// TODO add default options
	if err := v.VMStart(name, true, "", func(string) {}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func stopVM(c *gin.Context) {
	name := CFPrefix + c.Param("name")
	if err := v.VMStop(name); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func removeVM(c *gin.Context) {
	name := CFPrefix + c.Param("name")
	if err := v.VMRemove(name); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "ok"})
}

func getSystemImageList(c *gin.Context) {
	getFilesInFolder(c, ".zip", v.UploadDir)
}

func getCVDImageList(c *gin.Context) {
	getFilesInFolder(c, ".tar", v.UploadDir)
}

func getApkFileList(c *gin.Context) {
	containerName := CFPrefix + c.Param("name")
	getFilesInFolder(c, ".apk", path.Join(v.DevicesDir, containerName))
}

func getFilesInFolder(c *gin.Context, fileExtension string, folder string) {
	var files []string

	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, fileExtension) {
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

func uploadImageFile(c *gin.Context) {
	uploadFile(c, []string{".zip", ".tar"}, v.UploadDir)
}

func uploadDeviceFile(c *gin.Context) {
	containerName := CFPrefix + c.Param("name")
	uploadFile(c, []string{".apk"}, path.Join(v.DevicesDir, containerName))
}

func uploadFile(c *gin.Context, allowedExtensions []string, dstFolder string) {
	file, err := c.FormFile("file")
	// The file cannot be received.
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"message": "No file is received",
		})
		return
	}

	// Retrieve file information
	ext := filepath.Ext(file.Filename)

	for _, e := range allowedExtensions {
		if ext == e {
			// The file is received, so let's save it
			if err := c.SaveUploadedFile(file, path.Join(dstFolder, file.Filename)); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"message": "Unable to save the file",
				})
				return
			}

			// File saved successfully. Return proper result
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
			})
			return
		}
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"error": "Unsupported file formats"},
	)
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
