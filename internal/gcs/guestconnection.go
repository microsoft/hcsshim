//go:build windows

package gcs

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

const (
	protocolVersion = 4

	firstIoChannelVsockPort = prot.LinuxGcsVsockPort + 1
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
	ctx, span := oc.StartSpan(ctx, "gcs::GuestConnectionConfig::Connect", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

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
	caps       GuestDefinedCapabilities
	os         string
}

var _ cow.ProcessHost = &GuestConnection{}

// Capabilities returns the guest's declared capabilities.
func (gc *GuestConnection) Capabilities() GuestDefinedCapabilities {
	return gc.caps
}

// Protocol returns the protocol version that is in use.
func (gc *GuestConnection) Protocol() uint32 {
	return protocolVersion
}

// connect establishes a GCS connection. It must not be called more than once.
// isColdStart should be true when the UVM is being connected to for the first time post-boot.
// It should be false for subsequent connections (e.g. if reconnecting to an existing UVM).
func (gc *GuestConnection) connect(ctx context.Context, isColdStart bool, initGuestState *InitialGuestState) (err error) {
	req := prot.NegotiateProtocolRequest{
		MinimumVersion: protocolVersion,
		MaximumVersion: protocolVersion,
	}
	var resp prot.NegotiateProtocolResponse
	err = gc.brdg.RPC(ctx, prot.RpcNegotiateProtocol, &req, &resp, true)
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

	gc.caps, err = unmarshalGuestCapabilities(gc.os, resp.Capabilities.GuestDefinedCapabilities)
	if err != nil {
		return fmt.Errorf("unmarshalGuestCapabilities: %w", err)
	}

	if isColdStart && resp.Capabilities.SendHostCreateMessage {
		conf := &prot.UvmConfig{
			SystemType: "Container",
		}
		if initGuestState != nil && initGuestState.Timezone != nil {
			conf.TimeZoneInformation = initGuestState.Timezone
		}
		createReq := prot.ContainerCreate{
			RequestBase:     makeRequest(ctx, nullContainerID),
			ContainerConfig: prot.AnyInString{conf},
		}
		var createResp prot.ResponseBase
		err = gc.brdg.RPC(ctx, prot.RpcCreate, &createReq, &createResp, true)
		if err != nil {
			return err
		}
		if resp.Capabilities.SendHostStartMessage {
			startReq := makeRequest(ctx, nullContainerID)
			var startResp prot.ResponseBase
			err = gc.brdg.RPC(ctx, prot.RpcStart, &startReq, &startResp, true)
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
	ctx, span := oc.StartSpan(ctx, "gcs::GuestConnection::Modify", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	req := prot.ContainerModifySettings{
		RequestBase: makeRequest(ctx, nullContainerID),
		Request:     settings,
	}
	var resp prot.ResponseBase
	return gc.brdg.RPC(ctx, prot.RpcModifySettings, &req, &resp, false)
}

func (gc *GuestConnection) DumpStacks(ctx context.Context) (response string, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::GuestConnection::DumpStacks", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	req := prot.DumpStacksRequest{
		RequestBase: makeRequest(ctx, nullContainerID),
	}
	var resp prot.DumpStacksResponse
	err = gc.brdg.RPC(ctx, prot.RpcDumpStacks, &req, &resp, false)
	return resp.GuestStacks, err
}

func (gc *GuestConnection) DeleteContainerState(ctx context.Context, cid string) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::GuestConnection::DeleteContainerState", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", cid))

	req := prot.DeleteContainerStateRequest{
		RequestBase: makeRequest(ctx, cid),
	}
	var resp prot.ResponseBase
	return gc.brdg.RPC(ctx, prot.RpcDeleteContainerState, &req, &resp, false)
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
	ctx, span := oc.StartSpan(ctx, "gcs::GuestConnection::CreateProcess", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

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

func (gc *GuestConnection) notify(ntf *prot.ContainerNotification) error {
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

func makeRequest(ctx context.Context, cid string) prot.RequestBase {
	r := prot.RequestBase{
		ContainerID: cid,
	}
	span := trace.FromContext(ctx)
	if span != nil {
		sc := span.SpanContext()
		r.OpenCensusSpanContext = &prot.Ocspancontext{
			TraceID:      hex.EncodeToString(sc.TraceID[:]),
			SpanID:       hex.EncodeToString(sc.SpanID[:]),
			TraceOptions: uint32(sc.TraceOptions),
		}
		if sc.Tracestate != nil {
			entries := sc.Tracestate.Entries()
			if len(entries) > 0 {
				if bytes, err := json.Marshal(sc.Tracestate.Entries()); err == nil {
					r.OpenCensusSpanContext.Tracestate = base64.StdEncoding.EncodeToString(bytes)
				} else {
					log.G(ctx).WithError(err).Warn("failed to encode OpenCensus Tracestate")
				}
			}
		}
	}
	return r
}
