package hcsshim

import (
	"encoding/json"
	"io"
	"runtime"
	"syscall"

	"github.com/Sirupsen/logrus"
)

// CreateProcessParams is used as both the input of CreateProcessInComputeSystem
// and to convert the parameters to JSON for passing onto the HCS
type CreateProcessParams struct {
	ApplicationName  string
	CommandLine      string
	WorkingDirectory string
	Environment      map[string]string
	EmulateConsole   bool
	ConsoleSize      [2]int
}

// pipe struct used for the stdin/stdout/stderr pipes
type pipe struct {
	handle syscall.Handle
}

func makePipe(h syscall.Handle) *pipe {
	p := &pipe{h}
	runtime.SetFinalizer(p, (*pipe).closeHandle)
	return p
}

func (p *pipe) closeHandle() {
	if p.handle != 0 {
		syscall.CloseHandle(p.handle)
		p.handle = 0
	}
}

func (p *pipe) Close() error {
	p.closeHandle()
	runtime.SetFinalizer(p, nil)
	return nil
}

func (p *pipe) Read(b []byte) (int, error) {
	// syscall.Read returns 0, nil on ERROR_BROKEN_PIPE, but for
	// our purposes this should indicate EOF. This may be a go bug.
	var read uint32
	err := syscall.ReadFile(p.handle, b, &read, nil)
	if err != nil {
		if err == syscall.ERROR_BROKEN_PIPE {
			return 0, io.EOF
		}
		return 0, err
	}
	return int(read), nil
}

func (p *pipe) Write(b []byte) (int, error) {
	return syscall.Write(p.handle, b)
}

// CreateProcessInComputeSystem starts a process in a container. This is invoked, for example,
// as a result of docker run, docker exec, or RUN in Dockerfile. If successful,
// it returns the PID of the process.
func CreateProcessInComputeSystem(id string, useStdin bool, useStdout bool, useStderr bool, params CreateProcessParams) (uint32, io.WriteCloser, io.ReadCloser, io.ReadCloser, uint32, error) {
	title := "HCSShim::CreateProcessInComputeSystem"
	logrus.Debugf(title+" id=%s", id)

	// If we are not emulating a console, ignore any console size passed to us
	if !params.EmulateConsole {
		params.ConsoleSize[0] = 0
		params.ConsoleSize[1] = 0
	}

	paramsJson, err := json.Marshal(params)
	if err != nil {
		return 0, nil, nil, nil, 0xFFFFFFFF, err
	}

	var pid uint32

	logrus.Debugf(title+" - Calling Win32 %s %s", id, paramsJson)

	var stdinHandle, stdoutHandle, stderrHandle syscall.Handle
	var stdinParam, stdoutParam, stderrParam *syscall.Handle
	if useStdin {
		stdinParam = &stdinHandle
	}
	if useStdout {
		stdoutParam = &stdoutHandle
	}
	if useStderr {
		stderrParam = &stderrHandle
	}

	err = createProcessWithStdHandlesInComputeSystem(id, string(paramsJson), &pid, stdinParam, stdoutParam, stderrParam)
	if err != nil {
		err := makeErrorf(err, title, "id=%s params=%v", id, params)
		// Windows TP4: Hyper-V Containers may return this error with more than one
		// concurrent exec. Do not log it as an error
		if err.HResult() != Win32InvalidArgument {
			logrus.Error(err)
		}
		return 0, nil, nil, nil, err.HResult(), err
	}

	var (
		stdin          io.WriteCloser
		stdout, stderr io.ReadCloser
	)

	if useStdin {
		stdin = makePipe(stdinHandle)
	}
	if useStdout {
		stdout = makePipe(stdoutHandle)
	}
	if useStderr {
		stderr = makePipe(stderrHandle)
	}

	logrus.Debugf(title+" - succeeded id=%s params=%s pid=%d", id, paramsJson, pid)
	return pid, stdin, stdout, stderr, 0, nil
}
