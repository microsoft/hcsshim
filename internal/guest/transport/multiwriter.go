//go:build linux

package transport

import (
	"fmt"
	"io"
)

// multiWriter writes to both the underlying connection and the [io.Writer], w, via [io.multiWriter].
type multiWriter struct {
	t Transport
	w io.Writer
}

var _ Transport = &multiWriter{}

func NewMultiWriter(t Transport, w io.Writer) Transport {
	return &multiWriter{t, w}
}

// Dial accepts a vsock socket port number as configuration, and
// returns an unconnected VsockConnection struct.
func (t *multiWriter) Dial(port uint32) (Connection, error) {
	if t == nil || t.w == nil || t.t == nil {
		return nil, fmt.Errorf("invalid transpot")
	}

	conn, err := t.t.Dial(port)
	if err != nil {
		return nil, fmt.Errorf("multiwriter base transport dial: %w", err)
	}
	return &multiWriterConnection{conn, io.MultiWriter(conn, t.w)}, nil
}

type multiWriterConnection struct {
	Connection
	multi io.Writer
}

var _ Connection = &multiWriterConnection{}

func (c *multiWriterConnection) Write(buf []byte) (int, error) {
	return c.multi.Write(buf)
}
