//go:build linux
// +build linux

package bridge

import (
	"context"
	"encoding/json"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
)

// The capabilities of this GCS.
var capabilities = prot.GcsCapabilities{
	SendHostCreateMessage:   false,
	SendHostStartMessage:    false,
	HVSocketConfigOnStartup: false,
	SupportedSchemaVersions: []prot.SchemaVersion{
		{
			Major: 2,
			Minor: 1,
		},
	},
	RuntimeOsType: prot.OsTypeLinux,
	GuestDefinedCapabilities: prot.GcsGuestCapabilities{
		NamespaceAddRequestSupported:  true,
		SignalProcessSupported:        true,
		DumpStacksSupported:           true,
		DeleteContainerStateSupported: true,
	},
}

// negotiateProtocolV2 was introduced in v4 so will not be called with a minimum
// lower than that.
func (b *Bridge) negotiateProtocolV2(r *Request) (_ RequestResponse, err error) {
	_, span := oc.StartSpan(r.Context, "opengcs::bridge::negotiateProtocolV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.NegotiateProtocol
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	if request.MaximumVersion < uint32(prot.PvV4) || uint32(prot.PvMax) < request.MinimumVersion {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeUnsupportedProtocolVersion)
	}

	min := func(x, y uint32) uint32 {
		if x < y {
			return x
		}
		return y
	}

	major := min(uint32(prot.PvMax), request.MaximumVersion)

	// Set our protocol selected version before return.
	b.protVer = prot.ProtocolVersion(major)

	return &prot.NegotiateProtocolResponse{
		Version:      major,
		Capabilities: capabilities,
	}, nil
}

// createContainerV2 creates a container based on the settings passed in `r`.
//
// This is allowed only for protocol version 4+, schema version 2.1+
func (b *Bridge) createContainerV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::createContainerV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.ContainerCreate
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	var settingsV2 prot.VMHostedContainerSettingsV2
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.ContainerConfig), &settingsV2); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for ContainerConfig \"%s\"", request.ContainerConfig)
	}

	if settingsV2.SchemaVersion.Cmp(prot.SchemaVersion{Major: 2, Minor: 1}) < 0 {
		return nil, gcserr.WrapHresult(
			errors.Errorf("invalid schema version: %v", settingsV2.SchemaVersion),
			gcserr.HrVmcomputeInvalidJSON)
	}

	c, err := b.hostState.CreateContainer(ctx, request.ContainerID, &settingsV2)
	if err != nil {
		return nil, err
	}
	waitFn := func() prot.NotificationType {
		return c.Wait()
	}

	go func() {
		nt := waitFn()
		notification := &prot.ContainerNotification{
			MessageBase: prot.MessageBase{
				ContainerID: request.ContainerID,
				ActivityID:  request.ActivityID,
			},
			Type:       nt,
			Operation:  prot.AoNone,
			Result:     0,
			ResultInfo: "",
		}
		b.PublishNotification(notification)
	}()

	return &prot.ContainerCreateResponse{}, nil
}

// startContainerV2 doesn't have a great correlation to LCOW. On Windows this is
// used to start the container silo. In Linux the container is the process so we
// wait until the exec process of the init process to actually issue the start.
//
// This is allowed only for protocol version 4+, schema version 2.1+
func (b *Bridge) startContainerV2(r *Request) (_ RequestResponse, err error) {
	_, span := oc.StartSpan(r.Context, "opengcs::bridge::startContainerV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	// This is just a noop, but needs to be handled so that an error isn't
	// returned to the HCS.
	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	return &prot.MessageResponseBase{}, nil
}

// execProcessV2 is used to execute three types of processes in the guest.
//
// 1. HostProcess. This is a process in the Host pid namespace that runs as
// root. It is signified by either `request.IsExternal` or `request.ContainerID
// == hcsv2.UVMContainerID`.
//
// 2. Container Init process. This is the init process of the created container.
// We use exec for this instead of `StartContainer` because the protocol does
// not pass in the appropriate std pipes for relaying the results until exec.
// Until this is called the container remains in the `created` state.
//
// 3. Container Exec process. This is a process that is run in the container's
// pid namespace.
//
// This is allowed only for protocol version 4+, schema version 2.1+
func (b *Bridge) execProcessV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::execProcessV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.ContainerExecuteProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	// The request contains a JSON string field which is equivalent to an
	// ExecuteProcessInfo struct.
	var params prot.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.Settings.ProcessParameters), &params); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters)
	}

	var conSettings stdio.ConnectionSettings
	if params.CreateStdInPipe {
		conSettings.StdIn = &request.Settings.VsockStdioRelaySettings.StdIn
	}
	if params.CreateStdOutPipe {
		conSettings.StdOut = &request.Settings.VsockStdioRelaySettings.StdOut
	}
	if params.CreateStdErrPipe {
		conSettings.StdErr = &request.Settings.VsockStdioRelaySettings.StdErr
	}

	pid, err := b.hostState.ExecProcess(ctx, request.ContainerID, params, conSettings)

	if err != nil {
		return nil, err
	}
	log.G(ctx).WithField("pid", pid).Debug("created process pid")
	return &prot.ContainerExecuteProcessResponse{
		ProcessID: uint32(pid),
	}, nil
}

// killContainerV2 is a user forced terminate of the container and all processes
// in the container. It is equivalent to sending SIGKILL to the init process and
// all exec'd processes.
//
// This is allowed only for protocol version 4+, schema version 2.1+
func (b *Bridge) killContainerV2(r *Request) (RequestResponse, error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::killContainerV2")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	return b.signalContainerShutdownV2(ctx, span, r, false)
}

// shutdownContainerV2 is a user requested shutdown of the container and all
// processes in the container. It is equivalent to sending SIGTERM to the init
// process and all exec'd processes.
//
// This is allowed only for protocol version 4+, schema version 2.1+
func (b *Bridge) shutdownContainerV2(r *Request) (RequestResponse, error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::shutdownContainerV2")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	return b.signalContainerShutdownV2(ctx, span, r, true)
}

// signalContainerV2 is not a handler func. It is called from either
// `killContainerV2` or `shutdownContainerV2` to deliver a SIGTERM or SIGKILL
// respectively
func (b *Bridge) signalContainerShutdownV2(ctx context.Context, span *trace.Span, r *Request, graceful bool) (_ RequestResponse, err error) {
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", r.ContainerID),
		trace.BoolAttribute("graceful", graceful),
	)

	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	// If this is targeting the UVM send the request to the host itself.
	if request.ContainerID == hcsv2.UVMContainerID {
		// We are asking to shutdown the UVM itself.
		// This is a destructive call. We do not respond to the HCS
		b.quitChan <- true
		b.hostState.Shutdown()
	} else {
		err = b.hostState.ShutdownContainer(ctx, request.ContainerID, graceful)
		if err != nil {
			return nil, err
		}
	}

	return &prot.MessageResponseBase{}, nil
}

func (b *Bridge) signalProcessV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::signalProcessV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.ContainerSignalProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	span.AddAttributes(
		trace.Int64Attribute("pid", int64(request.ProcessID)),
		trace.Int64Attribute("signal", int64(request.Options.Signal)))

	var signal syscall.Signal
	if request.Options.Signal == 0 {
		signal = unix.SIGKILL
	} else {
		signal = syscall.Signal(request.Options.Signal)
	}

	if err := b.hostState.SignalContainerProcess(ctx, request.ContainerID, request.ProcessID, signal); err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}

func (b *Bridge) getPropertiesV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::getPropertiesV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.ContainerGetProperties
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	var query prot.PropertyQuery
	if len(request.Query) != 0 {
		if err := json.Unmarshal([]byte(request.Query), &query); err != nil {
			e := gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
			return nil, errors.Wrapf(e, "The query could not be unmarshaled: '%s'", query)
		}
	}

	if request.ContainerID == hcsv2.UVMContainerID {
		return nil, errors.New("getPropertiesV2 is not supported against the UVM")
	}

	properties, err := b.hostState.GetProperties(ctx, request.ContainerID, query)
	if err != nil {
		return nil, err
	}

	propertyJSON := []byte("{}")
	if properties != nil {
		var err error
		propertyJSON, err = json.Marshal(properties)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%+v\"", properties)
		}
	}

	return &prot.ContainerGetPropertiesResponse{
		Properties: string(propertyJSON),
	}, nil
}

func (b *Bridge) waitOnProcessV2(r *Request) (_ RequestResponse, err error) {
	_, span := oc.StartSpan(r.Context, "opengcs::bridge::waitOnProcessV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	span.AddAttributes(
		trace.Int64Attribute("pid", int64(request.ProcessID)),
		trace.Int64Attribute("timeout-ms", int64(request.TimeoutInMs)))

	var exitCodeChan <-chan int
	var doneChan chan<- bool

	if request.ContainerID == hcsv2.UVMContainerID {
		p, err := b.hostState.GetExternalProcess(int(request.ProcessID))
		if err != nil {
			return nil, err
		}
		exitCodeChan, doneChan = p.Wait()
	} else {
		c, err := b.hostState.GetCreatedContainer(request.ContainerID)
		if err != nil {
			return nil, err
		}
		p, err := c.GetProcess(request.ProcessID)
		if err != nil {
			return nil, err
		}
		exitCodeChan, doneChan = p.Wait()
	}

	// If we timed out or if we got the exit code. Acknowledge we no longer want to wait.
	defer close(doneChan)

	var tc <-chan time.Time
	if request.TimeoutInMs != prot.InfiniteWaitTimeout {
		t := time.NewTimer(time.Duration(request.TimeoutInMs) * time.Millisecond)
		defer t.Stop()
		tc = t.C
	}
	select {
	case exitCode := <-exitCodeChan:
		return &prot.ContainerWaitForProcessResponse{
			ExitCode: uint32(exitCode),
		}, nil
	case <-tc:
		return nil, gcserr.NewHresultError(gcserr.HvVmcomputeTimeout)
	}
}

func (b *Bridge) resizeConsoleV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::resizeConsoleV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	span.AddAttributes(
		trace.Int64Attribute("pid", int64(request.ProcessID)),
		trace.Int64Attribute("height", int64(request.Height)),
		trace.Int64Attribute("width", int64(request.Width)))

	c, err := b.hostState.GetCreatedContainer(request.ContainerID)
	if err != nil {
		return nil, err
	}

	p, err := c.GetProcess(request.ProcessID)
	if err != nil {
		return nil, err
	}

	err = p.ResizeConsole(ctx, request.Height, request.Width)
	if err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}

func (b *Bridge) modifySettingsV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::modifySettingsV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	request, err := prot.UnmarshalContainerModifySettings(r.Message)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	err = b.hostState.ModifySettings(ctx, request.ContainerID, request.Request.(*guestrequest.ModificationRequest))
	if err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}

func (b *Bridge) dumpStacksV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::dumpStacksV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	stacks, err := b.hostState.GetStacks(ctx)
	if err != nil {
		return nil, err
	}
	return &prot.DumpStacksResponse{
		GuestStacks: stacks,
	}, nil
}

func (b *Bridge) deleteContainerStateV2(r *Request) (_ RequestResponse, err error) {
	ctx, span := oc.StartSpan(r.Context, "opengcs::bridge::deleteContainerStateV2")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(trace.StringAttribute("cid", r.ContainerID))

	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message)
	}

	c, err := b.hostState.GetCreatedContainer(request.ContainerID)
	if err != nil {
		return nil, err
	}
	// remove container state regardless of delete's success
	defer b.hostState.RemoveContainer(request.ContainerID)

	if err := c.Delete(ctx); err != nil {
		return nil, err
	}

	return &prot.MessageResponseBase{}, nil
}
