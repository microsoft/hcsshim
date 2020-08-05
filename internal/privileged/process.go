package privileged

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/guestrequest"
	"github.com/Microsoft/hcsshim/internal/log"
	"golang.org/x/sys/windows"
)

// JobProcess represents a process run in a job object.
type JobProcess struct {
	cmd            *exec.Cmd
	stdioLock      sync.Mutex
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	exitCode       int
	waitBlock      chan struct{}
	closedWaitOnce sync.Once
	waitError      error
}

var _ cow.Process = &JobProcess{}

func newProcess(cmd *exec.Cmd) *JobProcess {
	return &JobProcess{
		cmd:       cmd,
		waitBlock: make(chan struct{}),
	}
}

var signalMap = map[guestrequest.SignalValueWCOW]uint32{
	guestrequest.SignalValueWCOWCtrlC:        uint32(windows.CTRL_C_EVENT),
	guestrequest.SignalValueWCOWCtrlBreak:    uint32(windows.CTRL_BREAK_EVENT),
	guestrequest.SignalValueWCOWCtrlClose:    uint32(windows.CTRL_CLOSE_EVENT),
	guestrequest.SignalValueWCOWCtrlLogOff:   uint32(windows.CTRL_LOGOFF_EVENT),
	guestrequest.SignalValueWCOWCtrlShutdown: uint32(windows.CTRL_SHUTDOWN_EVENT),
}

// ResizeConsole - TODO (dcantah): Implement console support with new pseudo console api?
// Then revisit this.
func (p *JobProcess) ResizeConsole(ctx context.Context, width, height uint16) error {
	return nil
}

// Stdio returns the stdio pipes of the process
func (p *JobProcess) Stdio() (io.Writer, io.Reader, io.Reader) {
	return p.stdin, p.stdout, p.stderr
}

// Signal sends a signal to the process.
func (p *JobProcess) Signal(ctx context.Context, options interface{}) (bool, error) {
	log.G(ctx).Debug("sending ctrl-break signal to job process")
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.Pid())); err != nil {
		return false, fmt.Errorf("failed to send signal: %s", err)
	}
	// Process wasn't killed from the signal. Ping handles ctrl-break for example
	// and we can't issue ctrl-c with a new process group.
	if p.cmd.ProcessState == nil {
		if err := p.cmd.Process.Kill(); err != nil {
			return false, err
		}
	}
	return true, nil
}

// CloseStdin closes the stdin pipe of the process.
func (p *JobProcess) CloseStdin(ctx context.Context) error {
	p.stdioLock.Lock()
	defer p.stdioLock.Unlock()
	return p.stdin.Close()
}

// Wait waits for the process to exit. If the process has already exited returns
// the previous error (if any).
func (p *JobProcess) Wait() error {
	<-p.waitBlock
	return p.waitError
}

// Start starts the job object process
func (p *JobProcess) start() error {
	return p.cmd.Start()
}

// This should only be called once.
func (p *JobProcess) waitBackground(ctx context.Context) {
	log.G(ctx).WithField("pid", p.Pid()).Debug("waitBackground for JobProcess")
	var (
		err      error
		exitCode = -1
	)
	if p.cmd.Process == nil {
		err = errors.New("process has not been started")
	}

	// Wait for process to get signaled/exit/terminate/.
	err = p.cmd.Wait()

	// Wait closes the stdio pipes so theres no need to later on.
	p.stdin = nil
	p.stdout = nil
	p.stderr = nil

	// Process completed or was terminated by this point
	// This shouldnt be nil after wait but just to be safe..
	exitCode = p.cmd.ProcessState.ExitCode()
	p.closedWaitOnce.Do(func() {
		p.waitError = err
		p.exitCode = exitCode
		close(p.waitBlock)
	})
}

// ExitCode returns the exit code of the process. The process must have
// already terminated.
func (p *JobProcess) ExitCode() (int, error) {
	if p.cmd.ProcessState == nil || !p.cmd.ProcessState.Exited() {
		return -1, errors.New("process has not exited")
	}
	return p.cmd.ProcessState.ExitCode(), nil
}

// Pid returns the processes PID
func (p *JobProcess) Pid() int {
	if process := p.cmd.Process; process != nil {
		return process.Pid
	}
	return 0
}

// Close cleans up any state associated with the process but does not kill it.
func (p *JobProcess) Close() error {
	p.stdioLock.Lock()
	if p.stdin != nil {
		p.stdin.Close()
		p.stdin = nil
	}
	if p.stdout != nil {
		p.stdout.Close()
		p.stdout = nil
	}
	if p.stderr != nil {
		p.stderr.Close()
		p.stderr = nil
	}
	p.stdioLock.Unlock()

	p.closedWaitOnce.Do(func() {
		p.exitCode = -1
		p.waitError = errors.New("process already closed")
		close(p.waitBlock)
	})
	return nil
}

// Kill kills the running process. Go calls TerminateProcess under the hood.
func (p *JobProcess) Kill(ctx context.Context) (bool, error) {
	log.G(ctx).WithField("pid", p.Pid()).Debug("killing job process")
	if p.cmd.Process != nil {
		// If the process already exited ignore.
		if p.cmd.ProcessState == nil {
			if err := p.cmd.Process.Kill(); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}
