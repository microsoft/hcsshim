package gcs

import (
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

const pipeName = `\\.\pipe\iochannel-test`

func TestIoChannelClose(t *testing.T) {
	l, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		t.Fatal(err)
	}
	ioc := newIoChannel(l)
	defer ioc.Close()
	ch := make(chan error, 1)
	go func() {
		_, err := ioutil.ReadAll(ioc)
		ch <- err
	}()
	time.Sleep(100 * time.Millisecond)
	ioc.Close()
	if err := <-ch; err == nil || !strings.Contains(err.Error(), "use of closed network connection") {
		t.Error("unexpected: ", err)
	}
}

func TestIoChannelRead(t *testing.T) {
	l, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		t.Fatal(err)
	}
	ioc := newIoChannel(l)
	defer ioc.Close()
	var b []byte
	ch := make(chan error, 1)
	go func() {
		var err error
		b, err = ioutil.ReadAll(ioc)
		ch <- err
	}()
	time.Sleep(100 * time.Millisecond)
	c, err := winio.DialPipe(pipeName, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Write(([]byte)("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err := <-ch; err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello world" {
		t.Errorf("unexpected: %q", string(b))
	}
}

func TestIoChannelWrite(t *testing.T) {
	l, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		t.Fatal(err)
	}
	ioc := newIoChannel(l)
	defer ioc.Close()
	ch := make(chan error, 1)
	go func() {
		_, err := ioc.Write(([]byte)("hello world"))
		ioc.Close()
		ch <- err
	}()
	time.Sleep(100 * time.Millisecond)
	c, err := winio.DialPipe(pipeName, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	b, err := ioutil.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-ch; err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello world" {
		t.Errorf("unexpected: %q", string(b))
	}
}
