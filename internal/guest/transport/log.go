//go:build linux

package transport

import (
	"github.com/sirupsen/logrus"
)

// logConnection wraps the underlying [Connection] and logs the Close*() operations with
// the connection's port number.
type logConnection struct {
	Connection
	entry *logrus.Entry
}

var _ Connection = &logConnection{}

func NewLogConnection(c Connection, port uint32) Connection {
	return &logConnection{c, logrus.WithField("port", port)}
}

func (c *logConnection) Close() error {
	c.entry.Debug("opengcs::logConnection::Close - closing connection")

	return c.Connection.Close()
}

func (c *logConnection) CloseRead() error {
	c.entry.Debug("opengcs::logConnection::Close - closing read connection")

	return c.Connection.CloseRead()
}

func (c *logConnection) CloseWrite() error {
	c.entry.Debug("opengcs::logConnection::Close - closing write connection")

	return c.Connection.CloseWrite()
}
