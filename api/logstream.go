package main

import (
	"io"
	"log"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"sea.com/matrisea/vmm"
)

func LogStreamHandler(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer conn.Close()

	containerName := CFPrefix + c.Param("name")

	var logFile string
	switch c.Param("source") {
	case "launcher":
		logFile = path.Join(vmm.HomeDir, "cuttlefish_runtime/launcher.log")
	case "kernel":
		logFile = path.Join(vmm.HomeDir, "cuttlefish_runtime/kernel.log")
	case "logcat":
		logFile = path.Join(vmm.HomeDir, "cuttlefish_runtime/logcat")
	default:
		log.Printf("Invalid log source %s on %s", c.Param("source"), containerName)
		return
	}

	cmd := []string{"tail", "-n", "2000", "-f", logFile}
	// run bash in container and get the hijacked session
	hijackedResp, err := v.ExecAttachToTTYProcess(containerName, cmd, []string{})
	if err != nil {
		log.Println("Failed to get log due to", err.Error())
		return
	}

	// clean up after quit
	defer func() {
		hijackedResp.Conn.Write([]byte("exit\r"))
		if err := v.KillTTYProcess(containerName, strings.Join(cmd, " ")); err != nil {
			log.Printf("Failed to kill log writer %s of container %s on exit due to %s", logFile, containerName, err.Error())
		}
	}()
	defer hijackedResp.Close()

	// forward read/write to websocket
	go wsLogWriterCopy(conn, hijackedResp.Conn)
	// Why wsReaderCopy here is not invoked as goroutine is to use client ws close event (e.g. browser tab closed)
	// as a signal of the end of user interaction, so we can trigger the deferred cleanup function.
	//
	// Sequence of events:
	//   --Start wsReaderCopy
	//   --Error in wsReaderCopy - socket: close 1001 (going away)
	//   --End of attach to terminal
	//   --Deferred cleanup
	wsLogReaderCopy(conn, hijackedResp.Conn)
}

func wsLogWriterCopy(writer *websocket.Conn, reader io.Reader) {
	buf := make([]byte, 8192)
	for {
		nr, err := reader.Read(buf)
		if err != nil {
			return
		}
		if nr > 0 {
			cleanLog := strings.ReplaceAll(string(buf[0:nr]), "\r", "")
			err := writer.WriteMessage(websocket.TextMessage, []byte(cleanLog))
			if err != nil {
				return
			}
		}
	}
}

// send front end input to terminal
func wsLogReaderCopy(reader *websocket.Conn, writer io.Writer) {
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
