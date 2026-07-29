//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	guestRuntime "github.com/Microsoft/hcsshim/internal/guest/runtime"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

var errTestContainerStart = errors.New("container start failed")

type startOrderContainer struct {
	guestRuntime.Container
	pipeRelay                     *stdio.PipeRelay
	ttyRelay                      *stdio.TtyRelay
	startErr                      error
	readStarted                   <-chan struct{}
	relayStartedBeforeStartReturn bool
}

func (c *startOrderContainer) Start() error {
	select {
	case <-c.readStarted:
		c.relayStartedBeforeStartReturn = true
	case <-time.After(500 * time.Millisecond):
	}
	return c.startErr
}

func (c *startOrderContainer) PipeRelay() *stdio.PipeRelay {
	return c.pipeRelay
}

func (c *startOrderContainer) Tty() *stdio.TtyRelay {
	return c.ttyRelay
}

type startSignalTransport struct {
	conn transport.Connection
}

func (t *startSignalTransport) Dial(uint32) (transport.Connection, error) {
	return t.conn, nil
}

type startSignalConnection struct {
	readStarted chan struct{}
	readOnce    sync.Once
	closed      chan struct{}
	closeOnce   sync.Once
}

func (c *startSignalConnection) Read([]byte) (int, error) {
	c.readOnce.Do(func() { close(c.readStarted) })
	return 0, io.EOF
}

func (c *startSignalConnection) Write(p []byte) (int, error) { return len(p), nil }
func (c *startSignalConnection) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}
func (c *startSignalConnection) CloseRead() error  { return nil }
func (c *startSignalConnection) CloseWrite() error { return nil }
func (c *startSignalConnection) File() (*os.File, error) {
	return nil, errors.New("startSignalConnection does not support File")
}

// TestContainerStartOrdersRelayAfterRuntimeStart verifies a failed runtime
// start cannot race relay connection teardown.
func TestContainerStartOrdersRelayAfterRuntimeStart(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal bool
		startErr error
	}{
		{name: "pipe/success"},
		{name: "pipe/failure", startErr: errTestContainerStart},
		{name: "tty/success", terminal: true},
		{name: "tty/failure", terminal: true, startErr: errTestContainerStart},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				pipeRelay *stdio.PipeRelay
				ttyRelay  *stdio.TtyRelay
			)
			if tc.terminal {
				pty, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
				if err != nil {
					t.Fatalf("open pty: %v", err)
				}
				ttyRelay = stdio.NewTtyRelay(nil, pty)
			} else {
				var err error
				pipeRelay, err = stdio.NewPipeRelay(nil)
				if err != nil {
					t.Fatalf("NewPipeRelay: %v", err)
				}
			}

			readStarted := make(chan struct{})
			closed := make(chan struct{})
			runtimeContainer := &startOrderContainer{
				pipeRelay:   pipeRelay,
				ttyRelay:    ttyRelay,
				startErr:    tc.startErr,
				readStarted: readStarted,
			}
			container := &Container{
				vsock: &startSignalTransport{conn: &startSignalConnection{
					readStarted: readStarted,
					closed:      closed,
				}},
				container:   runtimeContainer,
				initProcess: &containerProcess{spec: &oci.Process{Terminal: tc.terminal}, pid: 1},
			}

			port := uint32(1)
			_, err := container.Start(context.Background(), stdio.ConnectionSettings{StdIn: &port})
			if !errors.Is(err, tc.startErr) {
				t.Fatalf("Container.Start error = %v, want %v", err, tc.startErr)
			}
			if runtimeContainer.relayStartedBeforeStartReturn {
				t.Fatal("relay started before runtime container Start returned")
			}

			if tc.startErr != nil {
				select {
				case <-closed:
				default:
					t.Fatal("connection was not closed after runtime container Start failed")
				}
			} else {
				select {
				case <-readStarted:
				case <-time.After(5 * time.Second):
					t.Fatal("relay did not start after runtime container Start returned")
				}
				if ttyRelay != nil {
					ttyRelay.Wait()
				} else {
					pipeRelay.Wait()
				}
			}
		})
	}
}
