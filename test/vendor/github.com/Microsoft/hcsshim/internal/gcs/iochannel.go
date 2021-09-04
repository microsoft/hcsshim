package gcs

import (
	"net"
)

type ioChannel struct {
	l   net.Listener
	c   net.Conn
	err error
	ch  chan struct{}
}

func newIoChannel(l net.Listener) *ioChannel {
	c := &ioChannel{
		l:  l,
		ch: make(chan struct{}),
	}
	go c.accept()
	return c
}

func (c *ioChannel) accept() {
	c.c, c.err = c.l.Accept()
	c.l.Close()
	close(c.ch)
}

func (c *ioChannel) Close() error {
	if c == nil {
		return nil
	}
	c.l.Close()
	<-c.ch
	if c.c != nil {
		c.c.Close()
	}
	return nil
}

type closeWriter interface {
	CloseWrite() error
}

func (c *ioChannel) CloseWrite() error {
	<-c.ch
	if c.c == nil {
		return c.err
	}
	return c.c.(closeWriter).CloseWrite()
}

func (c *ioChannel) Read(b []byte) (int, error) {
	<-c.ch
	if c.c == nil {
		return 0, c.err
	}
	return c.c.Read(b)
}

func (c *ioChannel) Write(b []byte) (int, error) {
	<-c.ch
	if c.c == nil {
		return 0, c.err
	}
	return c.c.Write(b)
}
