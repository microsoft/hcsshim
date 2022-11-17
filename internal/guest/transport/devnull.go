//go:build linux
// +build linux

package transport

import (
	"os"

	"github.com/sirupsen/logrus"
)

type DevNullTransport struct{}

func (t *DevNullTransport) Dial(fd uint32) (Connection, error) {
	logrus.WithFields(logrus.Fields{
		"fd": fd,
	}).Info("opengcs::DevNullTransport::Dial")

	return newDevNullConnection(), nil
}

type devNullConnection struct {
	read  *os.File
	write *os.File
}

func newDevNullConnection() *devNullConnection {
	r, _ := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	w, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)

	return &devNullConnection{read: r, write: w}
}

func (c *devNullConnection) Close() error {
	err1 := c.read.Close()
	err2 := c.write.Close()

	if err1 != nil {
		return err1
	}

	if err2 != nil {
		return err2
	}

	return nil
}

func (c *devNullConnection) CloseRead() error {
	return c.read.Close()
}

func (c *devNullConnection) CloseWrite() error {
	return c.write.Close()
}

func (c *devNullConnection) Read(buf []byte) (int, error) {
	return c.read.Read(buf)
}

func (c *devNullConnection) Write(buf []byte) (int, error) {
	return c.write.Write(buf)
}

func (c *devNullConnection) File() (*os.File, error) {
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	return f, nil
}
