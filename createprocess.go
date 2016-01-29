package hcsshim

import (
	"encoding/json"
	"io"
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
		stdin, err = makeWin32File(stdinHandle)
	}
	if useStdout && err == nil {
		stdout, err = makeWin32File(stdoutHandle)
	}
	if useStderr && err == nil {
		stderr, err = makeWin32File(stderrHandle)
	}
	if err != nil {
		return 0, nil, nil, nil, 0xFFFFFFFF, err
	}

	logrus.Debugf(title+" - succeeded id=%s params=%s pid=%d", id, paramsJson, pid)
	return pid, stdin, stdout, stderr, 0, nil
}
