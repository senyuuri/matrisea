package main

import (
	"io"
	"log"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"sea.com/matrisea/vmm"
)

var (
	LOG_BUF_SIZE = 1024 * 128
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
	hijackedResp, err := v.ContainerAttachToProcess(containerName, cmd, []string{})
	if err != nil {
		log.Println("Failed to get log due to", err.Error())
		return
	}

	// clean up after quit
	defer func() {
		hijackedResp.Conn.Write([]byte("exit\r"))
		if err := v.ContainerKillProcess(containerName, strings.Join(cmd, " ")); err != nil {
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

type LogStream struct {
	buf    string
	length int
}

// Buffer log and send in batches. The log is flushed to WS writer when either
// (sendBuf is full) OR (sendBuf isn't full && timer's up && there's unsent log in sendBuf)
func wsLogWriterCopy(writer *websocket.Conn, reader io.Reader) {
	readBuf := make([]byte, LOG_BUF_SIZE)
	sendBuf := ""
	ch := make(chan LogStream)

	go func() {
		defer close(ch)
		for {
			nr, err := reader.Read(readBuf)
			if err != nil {
				return
			}
			if nr > 0 {
				cleanLog := strings.ReplaceAll(string(readBuf[0:nr]), "\r", "")
				ch <- LogStream{
					buf:    cleanLog,
					length: nr,
				}
			}
		}
	}()

	for {
		select {
		case logStream, ok := <-ch:
			if !ok {
				return
			}
			sendBuf = sendBuf + logStream.buf
			// fmt.Printf("sendBuf size %d, log size %d, is_sending %t\n", len(sendBuf), logStream.length, len(sendBuf) > LOG_BUF_SIZE)
			if len(sendBuf) > LOG_BUF_SIZE {
				err := writer.WriteMessage(websocket.TextMessage, []byte(sendBuf))
				if err != nil {
					return
				}
				sendBuf = ""
				// fmt.Printf("Full send. Reset sendBuf size %d\n", len(sendBuf))
			}
		case <-time.After(2 * time.Second):
			// process whatever we have seen so far if the batch size isn't filled in 3 secs
			if len(sendBuf) != 0 {
				err := writer.WriteMessage(websocket.TextMessage, []byte(sendBuf))
				if err != nil {
					return
				}
				sendBuf = ""
				// fmt.Printf("Timeout send. Reset sendBuf size %d\n", len(sendBuf))
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
