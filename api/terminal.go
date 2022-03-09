package main

import (
	"io"
	"log"
	"strconv"
	"strings"

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
	ir, hijackedResp, err := v.ContainerAttachToTerminal(containerName)
	if err != nil {
		log.Printf("%s: failed to attach to terminal: %v\n", containerName, err.Error())
		conn.WriteMessage(websocket.TextMessage, []byte("internal error:"+err.Error()))
		return
	}

	// clean up after quit
	defer func() {
		hijackedResp.Conn.Write([]byte("exit\r"))
		if err := v.ContainerKillTerminal(containerName); err != nil {
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
	wsReaderCopy(conn, hijackedResp.Conn, containerName, ir.ID)
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

// wsReaderCopy forwards front end input to the terminal.
func wsReaderCopy(reader *websocket.Conn, writer io.Writer, containerName string, execID string) {
	for {
		messageType, p, err := reader.ReadMessage()
		if err != nil {
			return
		}
		if messageType == websocket.TextMessage {
			if strings.HasPrefix(string(p), "$$MATRISEA_RESIZE") {
				s := strings.Split(string(p), " ")
				cols, err := strconv.ParseUint(s[1], 10, 64)
				if err != nil {
					log.Printf("%s: failed to parse resize cmd: %s\n", containerName, string(p))
					continue
				}
				lines, err := strconv.ParseUint(s[2], 10, 64)
				if err != nil {
					log.Printf("%s: failed to parse resize cmd: %s\n", containerName, string(p))
					continue
				}

				log.Printf("resize %s to %d, %d\n", containerName, cols, lines)
				v.ContainerTerminalResize(execID, uint(lines), uint(cols))
				continue
			}
			// Pass user input to the terminal
			writer.Write(p)
		}
	}
}
