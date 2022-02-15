package main

import (
	"io"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TerminalHandler(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer conn.Close()

	// read container name from URL params
	containerName := CFPrefix + c.Param("name")
	// run bash in container and get the hijacked session
	hijackedResp, err := v.ExecAttachToTerminal(containerName)
	if err != nil {
		// TODO how to let front end know this error?
		log.Println(err.Error())
		return
	}

	// clean up after quit
	defer func() {
		hijackedResp.Conn.Write([]byte("exit\r"))
		if err := v.KillTerminal(containerName); err != nil {
			log.Printf("Failed to kill terminal of container %s on exit due to %s", containerName, err.Error())
		}
	}()
	defer hijackedResp.Close()

	// forward read/write to websocket
	go wsWriterCopy(conn, hijackedResp.Conn)
	// Why wsReaderCopy here is not invoked as goroutine is to use client ws close event (e.g. browser tab closed)
	// as a signal of the end of user interaction, so we can trigger the deferred cleanup function.
	//
	// Sequence of events:
	//   --Start wsReaderCopy
	//   --Error in wsReaderCopy - socket: close 1001 (going away)
	//   --End of attach to terminal
	//   --Deferred cleanup
	wsReaderCopy(conn, hijackedResp.Conn)
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
