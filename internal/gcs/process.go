//go:build windows

package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

const (
	hrNotFound = 0x80070490
)

// Process represents a process in a container or container host.
type Process struct {
	gc                    *GuestConnection
	cid                   string
	id                    uint32
	waitCall              *rpc
	waitResp              prot.ContainerWaitForProcessResponse
	stdin, stdout, stderr *ioChannel
	stdinCloseWriteOnce   sync.Once
	stdinCloseWriteErr    error
}

var _ cow.Process = &Process{}

type baseProcessParams struct {
	CreateStdInPipe, CreateStdOutPipe, CreateStdErrPipe bool
}

func (gc *GuestConnection) exec(ctx context.Context, cid string, params interface{}) (_ cow.Process, err error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var bp baseProcessParams
	err = json.Unmarshal(b, &bp)
	if err != nil {
		return nil, err
	}

	req := prot.ContainerExecuteProcess{
		RequestBase: makeRequest(ctx, cid),
		Settings: prot.ExecuteProcessSettings{
			ProcessParameters: prot.AnyInString{params},
		},
	}

	p := &Process{gc: gc, cid: cid}
	defer func() {
		if err != nil {
			p.Close()
		}
	}()

	// Construct the stdio channels. Windows guests expect hvsock service IDs
	// instead of vsock ports.
	var hvsockSettings prot.ExecuteProcessStdioRelaySettings
	var vsockSettings prot.ExecuteProcessVsockStdioRelaySettings
	if gc.os == "windows" {
		req.Settings.StdioRelaySettings = &hvsockSettings
	} else {
		req.Settings.VsockStdioRelaySettings = &vsockSettings
	}
	if bp.CreateStdInPipe {
		p.stdin, vsockSettings.StdIn, err = gc.newIoChannel()
		if err != nil {
			return nil, err
		}
		g := winio.VsockServiceID(vsockSettings.StdIn)
		hvsockSettings.StdIn = &g
	}
	if bp.CreateStdOutPipe {
		p.stdout, vsockSettings.StdOut, err = gc.newIoChannel()
		if err != nil {
			return nil, err
		}
		g := winio.VsockServiceID(vsockSettings.StdOut)
		hvsockSettings.StdOut = &g
	}
	if bp.CreateStdErrPipe {
		p.stderr, vsockSettings.StdErr, err = gc.newIoChannel()
		if err != nil {
			return nil, err
		}
		g := winio.VsockServiceID(vsockSettings.StdErr)
		hvsockSettings.StdErr = &g
	}

	var resp prot.ContainerExecuteProcessResponse
	err = gc.brdg.RPC(ctx, prot.RpcExecuteProcess, &req, &resp, false)
	if err != nil {
		return nil, err
	}
	p.id = resp.ProcessID
	log.G(ctx).WithField("pid", p.id).Debug("created process pid")
	// Start a wait message.
	waitReq := prot.ContainerWaitForProcess{
		RequestBase: makeRequest(ctx, cid),
		ProcessID:   p.id,
		TimeoutInMs: 0xffffffff,
	}
	p.waitCall, err = gc.brdg.AsyncRPC(ctx, prot.RpcWaitForProcess, &waitReq, &p.waitResp)
	if err != nil {
		return nil, fmt.Errorf("failed to wait on process, leaking process: %w", err)
	}
	go p.waitBackground()
	return p, nil
}

// Close releases resources associated with the process and closes the
// associated standard IO streams.
func (p *Process) Close() error {
	ctx, span := oc.StartSpan(context.Background(), "gcs::Process::Close")
	defer span.End()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	if err := p.stdin.Close(); err != nil {
		log.G(ctx).WithError(err).Warn("close stdin failed")
	}
	if err := p.stdout.Close(); err != nil {
		log.G(ctx).WithError(err).Warn("close stdout failed")
	}
	if err := p.stderr.Close(); err != nil {
		log.G(ctx).WithError(err).Warn("close stderr failed")
	}
	return nil
}

// CloseStdin causes the process to read EOF on its stdin stream.
func (p *Process) CloseStdin(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Process::CloseStdin") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	p.stdinCloseWriteOnce.Do(func() {
		p.stdinCloseWriteErr = p.stdin.CloseWrite()
	})
	return p.stdinCloseWriteErr
}

func (p *Process) CloseStdout(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Process::CloseStdout") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	return p.stdout.Close()
}

func (p *Process) CloseStderr(ctx context.Context) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Process::CloseStderr") //nolint:ineffassign,staticcheck
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	return p.stderr.Close()
}

// ExitCode returns the process's exit code, or an error if the process is still
// running or the exit code is otherwise unknown.
func (p *Process) ExitCode() (_ int, err error) {
	if !p.waitCall.Done() {
		return -1, errors.New("process not exited")
	}
	if err := p.waitCall.Err(); err != nil {
		return -1, err
	}
	return int(p.waitResp.ExitCode), nil
}

// Kill sends a forceful terminate signal to the process and returns whether the
// signal was delivered. The process might not be terminated by the time this
// returns.
func (p *Process) Kill(ctx context.Context) (_ bool, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Process::Kill")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	return p.Signal(ctx, nil)
}

// Pid returns the process ID.
func (p *Process) Pid() int {
	return int(p.id)
}

// ResizeConsole requests that the pty associated with the process resize its
// window.
func (p *Process) ResizeConsole(ctx context.Context, width, height uint16) (err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Process::ResizeConsole", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	req := prot.ContainerResizeConsole{
		RequestBase: makeRequest(ctx, p.cid),
		ProcessID:   p.id,
		Height:      height,
		Width:       width,
	}
	var resp prot.ResponseBase
	return p.gc.brdg.RPC(ctx, prot.RpcResizeConsole, &req, &resp, true)
}

// Signal sends a signal to the process, returning whether it was delivered.
func (p *Process) Signal(ctx context.Context, options interface{}) (_ bool, err error) {
	ctx, span := oc.StartSpan(ctx, "gcs::Process::Signal", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	req := prot.ContainerSignalProcess{
		RequestBase: makeRequest(ctx, p.cid),
		ProcessID:   p.id,
		Options:     options,
	}
	var resp prot.ResponseBase
	// FUTURE: SIGKILL is idempotent and can safely be cancelled, but this interface
	//		   does currently make it easy to determine what signal is being sent.
	err = p.gc.brdg.RPC(ctx, prot.RpcSignalProcess, &req, &resp, false)
	if err != nil {
		if uint32(resp.Result) != hrNotFound {
			return false, err
		}
		if !p.waitCall.Done() {
			log.G(ctx).WithFields(logrus.Fields{
				logrus.ErrorKey:       err,
				logfields.ContainerID: p.cid,
				logfields.ProcessID:   p.id,
			}).Warn("ignoring missing process")
		}
		return false, nil
	}
	return true, nil
}

// Stdio returns the standard IO streams associated with the container. They
// will be closed when Close is called.
func (p *Process) Stdio() (stdin io.Writer, stdout, stderr io.Reader) {
	return p.stdin, p.stdout, p.stderr
}

// Wait waits for the process (or guest connection) to terminate.
func (p *Process) Wait() error {
	p.waitCall.Wait()
	return p.waitCall.Err()
}

func (p *Process) waitBackground() {
	ctx, span := oc.StartSpan(context.Background(), "gcs::Process::waitBackground")
	defer span.End()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.id)))

	p.waitCall.Wait()
	ec, err := p.ExitCode()
	if err != nil {
		log.G(ctx).WithError(err).Error("failed wait")
	}
	log.G(ctx).WithField("exitCode", ec).Debug("process exited")
	oc.SetSpanStatus(span, err)
}
