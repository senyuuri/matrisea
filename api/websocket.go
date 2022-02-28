package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// websocket - time allowed to read the next pong message from the peer
	pongWait = 10 * time.Second
	// websocket - send pings to peer with this period. Must be less than pongWait
	pingPeriod = 9 * time.Second
	// message size limit for websocket
	maxMessageSize int64 = 512
)

// Wrapper for gorilla/websocket's connection handler
type Connection struct {
	conn *websocket.Conn
	// buffered channel of outbound JSON message
	send    chan interface{}
	handler func(*Connection, []byte)
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Connection) readPump() {
	defer func() {
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, buf, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Failed to parse WS request. Reason: %s\n", err.Error())
			}
			return
		}
		// Handle the message in a new go routine so we won't block the readPump even if the
		// callee's gonna take a long time.
		// Without 'go', this function call will likely block PongHandler and cause the connection
		// to timeout.
		go c.handler(c, buf)
	}
}

// writePump pumps messages from the buffer channel to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			err := c.conn.WriteJSON(message)
			if err != nil {
				log.Println("writePump Error:", err.Error(), message)
				fmt.Println("writePump closed", err.Error())
				return
			}
		case <-ticker.C:
			// Send ping/pong message to keep websocket alive.
			// As per RFC, ping is sent by the server and the browser (not client code) should return pong.
			// Note that ping/pong message won't show up in Chrome devtools
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Set a handler to process message read from the connection
func (c *Connection) SetMessageHandler(h func(*Connection, []byte)) {
	if h == nil {
		h = func(*Connection, []byte) {
			log.Println("Error: websocket message handler not set")
		}
	}
	c.handler = h
}
