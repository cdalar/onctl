package providerboxes

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSConn is a WebSocket carrying a byte stream in binary messages, as a
// net.Conn -- so ssh (golang.org/x/crypto/ssh, or a ProxyCommand relay)
// can run over the boxes tunnel as if it were TCP.
type WSConn struct {
	ws *websocket.Conn

	readMu sync.Mutex
	cur    io.Reader // the message being read, if any

	writeMu sync.Mutex
}

var _ net.Conn = (*WSConn)(nil)

func NewWSConn(ws *websocket.Conn) *WSConn { return &WSConn{ws: ws} }

func (c *WSConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	for {
		if c.cur != nil {
			n, err := c.cur.Read(p)
			if errors.Is(err, io.EOF) {
				c.cur = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, err
		}
		_, r, err := c.ws.NextReader()
		if err != nil {
			var ce *websocket.CloseError
			if errors.As(err, &ce) {
				// The far side closing is the stream's end; a close with
				// a reason (box not running, port refused) is an error
				// worth showing.
				if ce.Code == websocket.CloseNormalClosure || ce.Text == "" {
					return 0, io.EOF
				}
				return 0, errors.New(ce.Text)
			}
			return 0, err
		}
		c.cur = r
	}
}

func (c *WSConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.ws.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close says goodbye properly, then closes the socket.
func (c *WSConn) Close() error {
	c.writeMu.Lock()
	_ = c.ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	c.writeMu.Unlock()
	return c.ws.Close()
}

func (c *WSConn) LocalAddr() net.Addr  { return c.ws.LocalAddr() }
func (c *WSConn) RemoteAddr() net.Addr { return c.ws.RemoteAddr() }

func (c *WSConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}
func (c *WSConn) SetReadDeadline(t time.Time) error  { return c.ws.SetReadDeadline(t) }
func (c *WSConn) SetWriteDeadline(t time.Time) error { return c.ws.SetWriteDeadline(t) }
