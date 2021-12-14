package jobcontainers

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/conpty"
	"github.com/Microsoft/hcsshim/internal/cow"
	"github.com/Microsoft/hcsshim/internal/exec"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/winapi"
)

// JobProcess represents a process run in a job object.
type JobProcess struct {
	cmd            *exec.Exec
	cpty           *conpty.Pty
	procLock       sync.Mutex
	stdioLock      sync.Mutex
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	waitBlock      chan struct{}
	closedWaitOnce sync.Once
	waitError      error
}

var sigMap = map[string]int{
	"CtrlC":        windows.CTRL_C_EVENT,
	"CtrlBreak":    windows.CTRL_BREAK_EVENT,
	"CtrlClose":    windows.CTRL_CLOSE_EVENT,
	"CtrlLogOff":   windows.CTRL_LOGOFF_EVENT,
	"CtrlShutdown": windows.CTRL_SHUTDOWN_EVENT,
}

var _ cow.Process = &JobProcess{}

func newProcess(cmd *exec.Exec, cpty *conpty.Pty) *JobProcess {
	return &JobProcess{
		cmd:       cmd,
		cpty:      cpty,
		waitBlock: make(chan struct{}),
	}
}

func (p *JobProcess) ResizeConsole(ctx context.Context, width, height uint16) error {
	if p.cpty == nil {
		return errors.New("no pseudo console assigned for process")
	}
	if err := p.cpty.Resize(int16(width), int16(height)); err != nil {
		return fmt.Errorf("failed to resize pseudo console for job container: %w", err)
	}
	return nil
}

// Stdio returns the stdio pipes of the process
func (p *JobProcess) Stdio() (io.Writer, io.Reader, io.Reader) {
	return p.stdin, p.stdout, p.stderr
}

// Signal sends a signal to the process and returns whether the signal was delivered.
func (p *JobProcess) Signal(ctx context.Context, options interface{}) (bool, error) {
	p.procLock.Lock()
	defer p.procLock.Unlock()

	if p.exited() {
		return false, errors.New("signal not sent. process has already exited")
	}

	// If options is nil it's assumed we got a sigterm
	if options == nil {
		if err := p.cmd.Kill(); err != nil {
			return false, err
		}
		return true, nil
	}

	signalOptions, ok := options.(*guestresource.SignalProcessOptionsWCOW)
	if !ok {
		return false, errors.New("unknown signal options")
	}

	signal, ok := sigMap[string(signalOptions.Signal)]
	if !ok {
		return false, fmt.Errorf("unknown signal %s encountered", signalOptions.Signal)
	}

	if err := signalProcess(uint32(p.cmd.Pid()), signal); err != nil {
		return false, errors.Wrap(err, "failed to send signal")
	}
	return true, nil
}

// CloseStdin closes the stdin pipe of the process.
func (p *JobProcess) CloseStdin(ctx context.Context) error {
	p.stdioLock.Lock()
	defer p.stdioLock.Unlock()
	if p.stdin != nil {
		if err := p.stdin.Close(); err != nil {
			return errors.Wrap(err, "failed to close job container stdin")
		}
		p.stdin = nil
	}
	return nil
}

// CloseStdout closes the stdout pipe of the process.
func (p *JobProcess) CloseStdout(ctx context.Context) error {
	p.stdioLock.Lock()
	defer p.stdioLock.Unlock()
	if p.stdout != nil {
		if err := p.stdout.Close(); err != nil {
			return errors.Wrap(err, "failed to close job container stdout")
		}
		p.stdout = nil
	}
	return nil
}

// CloseStderr closes the stderr pipe of the process.
func (p *JobProcess) CloseStderr(ctx context.Context) error {
	p.stdioLock.Lock()
	defer p.stdioLock.Unlock()
	if p.stderr != nil {
		if err := p.stderr.Close(); err != nil {
			return errors.Wrap(err, "failed to close job container stderr")
		}
		p.stderr = nil
	}
	return nil
}

// Wait waits for the process to exit. If the process has already exited returns
// the previous error (if any).
func (p *JobProcess) Wait() error {
	<-p.waitBlock
	return p.waitError
}

// Start starts the job object process
func (p *JobProcess) Start() error {
	return p.cmd.Start()
}

// This should only be called once.
func (p *JobProcess) waitBackground(ctx context.Context) {
	log.G(ctx).WithField("pid", p.Pid()).Debug("waitBackground for JobProcess")

	// Wait for process to get signaled/exit/terminate/.
	err := p.cmd.Wait()

	p.stdioLock.Lock()
	// Close the pseudo console if one was created for this process. Typical scenario is an exec with -it supplied.
	if p.cpty != nil {
		p.cpty.Close()
		p.cpty = nil
	}
	// Wait closes the stdio pipes so theres no need to here.
	p.stdin = nil
	p.stdout = nil
	p.stderr = nil
	p.stdioLock.Unlock()

	p.closedWaitOnce.Do(func() {
		p.waitError = err
		close(p.waitBlock)
	})
}

// ExitCode returns the exit code of the process.
func (p *JobProcess) ExitCode() (int, error) {
	p.procLock.Lock()
	defer p.procLock.Unlock()

	if !p.exited() {
		return -1, errors.New("process has not exited")
	}
	return p.cmd.ExitCode(), nil
}

// Pid returns the processes PID
func (p *JobProcess) Pid() int {
	return p.cmd.Pid()
}

// Close cleans up any state associated with the process but does not kill it.
func (p *JobProcess) Close() error {
	p.stdioLock.Lock()
	if p.cpty != nil {
		p.cpty.Close()
		p.cpty = nil
	}
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
		p.waitError = hcs.ErrAlreadyClosed
		close(p.waitBlock)
	})
	return nil
}

// Kill signals the process to terminate.
// Returns a bool signifying whether the signal was successfully delivered.
func (p *JobProcess) Kill(ctx context.Context) (bool, error) {
	log.G(ctx).WithField("pid", p.Pid()).Debug("killing job process")

	p.procLock.Lock()
	defer p.procLock.Unlock()

	if p.exited() {
		return false, errors.New("kill not sent. process already exited")
	}

	if err := p.cmd.Kill(); err != nil {
		return false, err
	}
	return true, nil
}

func (p *JobProcess) exited() bool {
	return p.cmd.Exited()
}

// signalProcess sends the specified signal to a process.
func signalProcess(pid uint32, signal int) error {
	hProc, err := windows.OpenProcess(winapi.PROCESS_ALL_ACCESS, true, pid)
	if err != nil {
		return errors.Wrap(err, "failed to open process")
	}
	defer func() {
		_ = windows.Close(hProc)
	}()

	// We can't use GenerateConsoleCtrlEvent since that only supports CTRL_C_EVENT and CTRL_BREAK_EVENT.
	// Instead, to handle an arbitrary signal we open a CtrlRoutine thread inside the target process and
	// give it the specified signal to handle. This is safe even with ASLR as even though kernel32.dll's
	// location will be randomized each boot, it will be in the same address for every process. This is why
	// we're able to get the address from a different process and use this as the start address for the routine
	// that the thread will run.
	//
	// Note: This is a hack which is not officially supported.
	k32, err := windows.LoadLibrary("kernel32.dll")
	if err != nil {
		return errors.Wrap(err, "failed to load kernel32 library")
	}
	defer func() {
		_ = windows.FreeLibrary(k32)
	}()

	proc, err := windows.GetProcAddress(k32, "CtrlRoutine")
	if err != nil {
		return errors.Wrap(err, "failed to load CtrlRoutine")
	}

	threadHandle, err := winapi.CreateRemoteThread(hProc, nil, 0, proc, uintptr(signal), 0, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to open remote thread in target process %d", pid)
	}
	defer func() {
		_ = windows.Close(threadHandle)
	}()
	return nil
}
