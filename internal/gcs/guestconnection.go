package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/schema1"
	"github.com/sirupsen/logrus"
)

const (
	protocolVersion = 4

	firstIoChannelVsockPort = LinuxGcsVsockPort + 1
	nullContainerID         = "00000000-0000-0000-0000-000000000000"
)

// IoListenFunc is a type for a function that creates a listener for a VM for
// the vsock port `port`.
type IoListenFunc func(port uint32) (net.Listener, error)

// HvsockIoListen returns an implementation of IoListenFunc that listens
// on the specified vsock port for the VM specified by `vmID`.
func HvsockIoListen(vmID guid.GUID) IoListenFunc {
	return func(port uint32) (net.Listener, error) {
		return winio.ListenHvsock(&winio.HvsockAddr{
			VMID:      vmID,
			ServiceID: winio.VsockServiceID(port),
		})
	}
}

// GuestConnectionConfig contains options for creating a guest connection.
type GuestConnectionConfig struct {
	// Conn specifies the connection to use for the bridge. It will be closed
	// when there is an error or Close is called.
	Conn io.ReadWriteCloser
	// Log specifies the logrus entry to use for async log messages.
	Log *logrus.Entry
	// IoListen is the function to use to create listeners for the stdio connections.
	IoListen IoListenFunc
}

// Connect establishes a GCS connection. `gcc.Conn` will be closed by this function.
func (gcc *GuestConnectionConfig) Connect(ctx context.Context) (*GuestConnection, error) {
	gc := &GuestConnection{
		nextPort:   firstIoChannelVsockPort,
		notifyChs:  make(map[string]chan struct{}),
		ioListenFn: gcc.IoListen,
	}
	gc.brdg = newBridge(gcc.Conn, gc.notify, gcc.Log)
	gc.brdg.Start()
	go func() {
		gc.brdg.Wait()
		gc.clearNotifies()
	}()
	err := gc.connect(ctx)
	if err != nil {
		gc.Close()
		return nil, err
	}
	return gc, nil
}

// GuestConnection represents a connection to the GCS.
type GuestConnection struct {
	brdg       *bridge
	ioListenFn IoListenFunc
	mu         sync.Mutex
	nextPort   uint32
	notifyChs  map[string]chan struct{}
	caps       schema1.GuestDefinedCapabilities
	os         string
}

var _ cow.ProcessHost = &GuestConnection{}

// Capabilities returns the guest's declared capabilities.
func (gc *GuestConnection) Capabilities() *schema1.GuestDefinedCapabilities {
	return &gc.caps
}

// Protocol returns the protocol version that is in use.
func (gc *GuestConnection) Protocol() uint32 {
	return protocolVersion
}

// connect establishes a GCS connection. It must not be called more than once.
func (gc *GuestConnection) connect(ctx context.Context) (err error) {
	req := negotiateProtocolRequest{
		MinimumVersion: protocolVersion,
		MaximumVersion: protocolVersion,
	}
	var resp negotiateProtocolResponse
	resp.Capabilities.GuestDefinedCapabilities = &gc.caps
	err = gc.brdg.RPC(ctx, rpcNegotiateProtocol, &req, &resp, true)
	if err != nil {
		return err
	}
	if resp.Version != protocolVersion {
		return fmt.Errorf("unexpected version %d returned", resp.Version)
	}
	gc.os = strings.ToLower(resp.Capabilities.RuntimeOsType)
	if gc.os == "" {
		gc.os = "windows"
	}
	if resp.Capabilities.SendHostCreateMessage {
		createReq := containerCreate{
			requestBase: makeRequest(nullContainerID),
			ContainerConfig: anyInString{&uvmConfig{
				SystemType: "Container",
			}},
		}
		var createResp responseBase
		err = gc.brdg.RPC(ctx, rpcCreate, &createReq, &createResp, true)
		if err != nil {
			return err
		}
		if resp.Capabilities.SendHostStartMessage {
			startReq := makeRequest(nullContainerID)
			var startResp responseBase
			err = gc.brdg.RPC(ctx, rpcStart, &startReq, &startResp, true)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Modify sends a modify settings request to the null container. This is
// generally used to prepare virtual hardware that has been added to the guest.
func (gc *GuestConnection) Modify(ctx context.Context, settings interface{}) error {
	req := containerModifySettings{
		requestBase: makeRequest(nullContainerID),
		Request:     settings,
	}
	var resp responseBase
	return gc.brdg.RPC(ctx, rpcModifySettings, &req, &resp, false)
}

// Close terminates the guest connection. It is undefined to call any other
// methods on the connection after this is called.
func (gc *GuestConnection) Close() error {
	if gc.brdg == nil {
		return nil
	}
	return gc.brdg.Close()
}

// CreateProcess creates a process in the container host.
func (gc *GuestConnection) CreateProcess(settings interface{}) (cow.Process, error) {
	return gc.exec(context.TODO(), nullContainerID, settings)
}

// OS returns the operating system of the container's host, "windows" or "linux".
func (gc *GuestConnection) OS() string {
	return gc.os
}

// IsOCI returns false, indicating that CreateProcess should not be called with
// an OCI process spec.
func (gc *GuestConnection) IsOCI() bool {
	return false
}

func (gc *GuestConnection) newIoChannel() (*ioChannel, uint32, error) {
	gc.mu.Lock()
	port := gc.nextPort
	gc.nextPort++
	gc.mu.Unlock()
	l, err := gc.ioListenFn(port)
	if err != nil {
		return nil, 0, err
	}
	return newIoChannel(l), port, nil
}

func (gc *GuestConnection) requestNotify(cid string, ch chan struct{}) error {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if gc.notifyChs == nil {
		return errors.New("guest connection closed")
	}
	if _, ok := gc.notifyChs[cid]; ok {
		return fmt.Errorf("container %s already exists", cid)
	}
	gc.notifyChs[cid] = ch
	return nil
}

func (gc *GuestConnection) notify(ntf *containerNotification) error {
	cid := ntf.ContainerID
	gc.mu.Lock()
	ch := gc.notifyChs[cid]
	delete(gc.notifyChs, cid)
	gc.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("container %s not found", cid)
	}
	logrus.WithField(logfields.ContainerID, cid).Info("container terminated in guest")
	close(ch)
	return nil
}

func (gc *GuestConnection) clearNotifies() {
	gc.mu.Lock()
	chs := gc.notifyChs
	gc.notifyChs = nil
	gc.mu.Unlock()
	for _, ch := range chs {
		close(ch)
	}
}

func makeRequest(cid string) requestBase {
	return requestBase{
		ContainerID: cid,
	}
}
