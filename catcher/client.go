package catcher

import (
	"time"

	"github.com/gorilla/websocket"
)

const outputChannelBuffer = 5
const pingFrequency = 10 * time.Second
const readDeadline = time.Minute
const maxMessageSize = 1024

type client struct {
	pingTicker time.Ticker
	catcher    *Catcher
	conn       *websocket.Conn
	output     chan interface{}
}

func newClient(catcher *Catcher, conn *websocket.Conn) *client {
	c := &client{
		catcher: catcher,
		conn:    conn,
		output:  make(chan interface{}, outputChannelBuffer),
	}
	go c.writeLoop()
	go c.readLoop()
	return c
}

func (c *client) Ping() error {
	return c.conn.WriteMessage(websocket.PingMessage, []byte{})
}

func (c *client) Close() error {
	return c.conn.WriteMessage(websocket.CloseMessage, []byte{})
}

func (c *client) SendJSON(obj interface{}) error {
	return c.conn.WriteJSON(obj)
}

func (c *client) writeLoop() {
	defer func() {
		c.catcher.logger.Info("Client %v exiting", c)
		c.pingTicker.Stop()
		c.conn.Close()
		delete(c.catcher.clients, c.conn)
	}()

	for {
		select {
		case <-c.pingTicker.C:
			if err := c.Ping(); err != nil {
				c.catcher.logger.Error("Error pinging: %v", err)
				return
			}
		case msg, ok := <-c.output:
			if !ok {
				c.Close()
				return
			}

			if err := c.SendJSON(msg); err != nil {
				c.catcher.logger.Error("Error sending message: %v", err)
				return
			}
		}
	}
}

func (c *client) readLoop() {
	// We don't care about what the client sends to us, but we need to
	// read it to keep the connection fresh.
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(readDeadline))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(readDeadline))
		return nil
	})
	for {
		if _, _, err := c.conn.NextReader(); err != nil {
			c.conn.Close()
			break
		}
	}
}
