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
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/sirupsen/logrus"
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
	waitResp              containerWaitForProcessResponse
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

	req := containerExecuteProcess{
		requestBase: makeRequest(cid),
		Settings: executeProcessSettings{
			ProcessParameters: anyInString{params},
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
	var hvsockSettings executeProcessStdioRelaySettings
	var vsockSettings executeProcessVsockStdioRelaySettings
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

	var resp containerExecuteProcessResponse
	err = gc.brdg.RPC(ctx, rpcExecuteProcess, &req, &resp, false)
	if err != nil {
		return nil, err
	}
	p.id = resp.ProcessID
	// Start a wait message.
	waitReq := containerWaitForProcess{
		requestBase: makeRequest(cid),
		ProcessID:   p.id,
		TimeoutInMs: 0xffffffff,
	}
	p.waitCall, err = gc.brdg.AsyncRPC(ctx, rpcWaitForProcess, &waitReq, &p.waitResp)
	if err != nil {
		return nil, fmt.Errorf("failed to wait on process, leaking process: %s", err)
	}
	return p, nil
}

// Close releases resources associated with the process and closes the
// associated standard IO streams.
func (p *Process) Close() error {
	p.stdin.Close()
	p.stdout.Close()
	p.stderr.Close()
	return nil
}

// CloseStdin causes the process to read EOF on its stdin stream.
func (p *Process) CloseStdin() (err error) {
	p.stdinCloseWriteOnce.Do(func() {
		p.stdinCloseWriteErr = p.stdin.CloseWrite()
	})
	return p.stdinCloseWriteErr
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
func (p *Process) Kill() (bool, error) {
	return p.Signal(nil)
}

// Pid returns the process ID.
func (p *Process) Pid() int {
	return int(p.id)
}

// ResizeConsole requests that the pty associated with the process resize its
// window.
func (p *Process) ResizeConsole(width, height uint16) (err error) {
	req := containerResizeConsole{
		requestBase: makeRequest(p.cid),
		ProcessID:   p.id,
		Height:      height,
		Width:       width,
	}
	var resp responseBase
	return p.gc.brdg.RPC(context.TODO(), rpcResizeConsole, &req, &resp, true)
}

// Signal sends a signal to the process, returning whether it was delivered.
func (p *Process) Signal(options interface{}) (bool, error) {
	req := containerSignalProcess{
		requestBase: makeRequest(p.cid),
		ProcessID:   p.id,
		Options:     options,
	}
	var resp responseBase
	// FUTURE: SIGKILL is idempotent and can safely be cancelled, but this interface
	//		   does currently make it easy to determine what signal is being sent.
	err := p.gc.brdg.RPC(context.TODO(), rpcSignalProcess, &req, &resp, false)
	if err != nil {
		if uint32(resp.Result) != hrNotFound {
			return false, err
		}
		if !p.waitCall.Done() {
			logrus.WithFields(logrus.Fields{
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
func (p *Process) Wait() (err error) {
	p.waitCall.Wait()
	return p.waitCall.Err()
}
