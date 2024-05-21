//go:build windows

package gcs

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/hcs/schema1"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

type InitialGuestState struct {
	// Timezone is only honored for Windows guests.
	Timezone *hcsschema.TimeZoneInformation
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
	// InitGuestState specifies settings to apply to the guest on creation/start. This includes things such as the timezone for the VM.
	InitGuestState *InitialGuestState
}

// Connect establishes a GCS connection. `gcc.Conn` will be closed by this function.
func (gcc *GuestConnectionConfig) Connect(ctx context.Context, isColdStart bool) (_ *GuestConnection, err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::GuestConnectionConfig::Connect", otelutil.WithClientSpanKind)
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	gc := &GuestConnection{
		nextPort:   firstIoChannelVsockPort,
		notifyChs:  make(map[string]chan struct{}),
		ioListenFn: gcc.IoListen,
	}
	gc.brdg = newBridge(gcc.Conn, gc.notify, gcc.Log)
	gc.brdg.Start()
	go func() {
		_ = gc.brdg.Wait()
		gc.clearNotifies()
	}()
	err = gc.connect(ctx, isColdStart, gcc.InitGuestState)
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
// isColdStart should be true when the UVM is being connected to for the first time post-boot.
// It should be false for subsequent connections (e.g. if reconnecting to an existing UVM).
func (gc *GuestConnection) connect(ctx context.Context, isColdStart bool, initGuestState *InitialGuestState) (err error) {
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
	if isColdStart && resp.Capabilities.SendHostCreateMessage {
		conf := &uvmConfig{
			SystemType: "Container",
		}
		if initGuestState != nil && initGuestState.Timezone != nil {
			conf.TimeZoneInformation = initGuestState.Timezone
		}
		createReq := containerCreate{
			requestBase:     makeRequest(ctx, nullContainerID),
			ContainerConfig: anyInString{conf},
		}
		var createResp responseBase
		err = gc.brdg.RPC(ctx, rpcCreate, &createReq, &createResp, true)
		if err != nil {
			return err
		}
		if resp.Capabilities.SendHostStartMessage {
			startReq := makeRequest(ctx, nullContainerID)
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
func (gc *GuestConnection) Modify(ctx context.Context, settings interface{}) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::GuestConnection::Modify", otelutil.WithClientSpanKind)
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := containerModifySettings{
		requestBase: makeRequest(ctx, nullContainerID),
		Request:     settings,
	}
	var resp responseBase
	return gc.brdg.RPC(ctx, rpcModifySettings, &req, &resp, false)
}

func (gc *GuestConnection) DumpStacks(ctx context.Context) (response string, err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::GuestConnection::DumpStacks", otelutil.WithClientSpanKind)
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := dumpStacksRequest{
		requestBase: makeRequest(ctx, nullContainerID),
	}
	var resp dumpStacksResponse
	err = gc.brdg.RPC(ctx, rpcDumpStacks, &req, &resp, false)
	return resp.GuestStacks, err
}

func (gc *GuestConnection) DeleteContainerState(ctx context.Context, cid string) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::GuestConnection::DeleteContainerState", otelutil.WithClientSpanKind, trace.WithAttributes(
		attribute.String("cid", cid)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	req := deleteContainerStateRequest{
		requestBase: makeRequest(ctx, cid),
	}
	var resp responseBase
	return gc.brdg.RPC(ctx, rpcDeleteContainerState, &req, &resp, false)
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
func (gc *GuestConnection) CreateProcess(ctx context.Context, settings interface{}) (_ cow.Process, err error) {
	ctx, span := otelutil.StartSpan(ctx, "gcs::GuestConnection::CreateProcess", otelutil.WithClientSpanKind)
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	return gc.exec(ctx, nullContainerID, settings)
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

func makeRequest(ctx context.Context, cid string) requestBase {
	r := requestBase{
		ContainerID: cid,
	}
	if span := trace.SpanFromContext(ctx); span != nil {
		r.SpanContext = span.SpanContext()
	}
	return r
}
