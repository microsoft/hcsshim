package uvm

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/internal/copywithtimeout"
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/schema1"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

// ByteCounts are the number of bytes copied to/from standard handles. Note
// this is int64 rather than uint64 to match the golang io.Copy() signature.
type ByteCounts struct {
	In  int64
	Out int64
	Err int64
}

// ProcessOptions are the set of options which are passed to CreateProcess() to
// create a utility vm.
type ProcessOptions struct {
	Process    *specs.Process
	Stdin      io.Reader  // Optional reader for sending on to the processes stdin stream
	Stdout     io.Writer  // Optional writer for returning the processes stdout stream
	Stderr     io.Writer  // Optional writer for returning the processes stderr stream
	ByteCounts ByteCounts // How much data to copy on each stream if they are supplied. 0 means to io.EOF.
}

// CreateProcess creates a process in the utility VM. This is only used
// on LCOW to run processes for remote filesystem commands, utilities, and debugging.
// It will return an error on a Windows utility VM.
//
// It optional performs IO copies with timeout between the pipes provided as input,
// and the pipes in the process.
//
// In the ProcessOptions structure, if byte-counts are non-zero, a maximum of those
// bytes are copied to the appropriate standard IO reader/writer. When zero,
// it copies until EOF. It also returns byte-counts indicating how much data
// was sent/received from the process.
//
// It is the responsibility of the caller to call Close() on the process returned.
//
// TODO: This could be removed as on LCOW as we could run a privileged container.

func (uvm *UtilityVM) CreateProcess(opts *ProcessOptions) (*hcs.Process, *ByteCounts, error) {
	if uvm.hcsSystem == nil {
		return nil, nil, fmt.Errorf("cannot CreateProcess - no hcsSystem handle")
	}
	if uvm.operatingSystem != "linux" {
		return nil, nil, fmt.Errorf("CreateProcess only supported on linux utility VMs")
	}

	if opts.Process == nil {
		return nil, nil, fmt.Errorf("no Process passed to CreateProcessEx")
	}

	copiedByteCounts := &ByteCounts{}
	commandLine := strings.Join(opts.Process.Args, " ")
	environment := make(map[string]string)
	for _, v := range opts.Process.Env {
		s := strings.SplitN(v, "=", 2)
		if len(s) == 2 && len(s[1]) > 0 {
			environment[s[0]] = s[1]
		}
	}

	if uvm.operatingSystem == "linux" {
		if _, ok := environment["PATH"]; !ok {
			environment["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:"
		}
	}

	processConfig := &schema1.ProcessConfig{
		EmulateConsole:    false,
		CreateStdInPipe:   (opts.Stdin != nil),
		CreateStdOutPipe:  (opts.Stdout != nil),
		CreateStdErrPipe:  (opts.Stderr != nil),
		CreateInUtilityVm: true,
		WorkingDirectory:  opts.Process.Cwd,
		Environment:       environment,
		CommandLine:       commandLine,
	}

	proc, err := uvm.hcsSystem.CreateProcess(processConfig)
	if err != nil {
		logrus.Debugf("failed to create process: %s", err)
		return nil, nil, err
	}

	processStdin, processStdout, processStderr, err := proc.Stdio()
	if err != nil {
		proc.Kill() // Should this have a timeout?
		proc.Close()
		return nil, nil, fmt.Errorf("failed to get stdio pipes for process %+v: %s", processConfig, err)
	}

	// TODO: The timeouts on the following Copies

	// Send the data into the process's stdin
	if opts.Stdin != nil {
		if copiedByteCounts.In, err = copywithtimeout.Copy(processStdin,
			opts.Stdin,
			opts.ByteCounts.In,
			fmt.Sprintf("CreateProcessEx: to stdin of %q", commandLine),
			time.Duration(4*time.Minute)); err != nil {
			return nil, nil, err
		}

		// Don't need stdin now we've sent everything. This signals GCS that we are finished sending data.
		if err := proc.CloseStdin(); err != nil && !hcs.IsNotExist(err) && !hcs.IsAlreadyClosed(err) {
			// This error will occur if the compute system is currently shutting down
			if perr, ok := err.(*hcs.ProcessError); ok && perr.Err != hcs.ErrVmcomputeOperationInvalidState {
				return nil, nil, err
			}
		}
	}

	// Copy the data back from stdout
	if opts.Stdout != nil {
		// Copy the data over to the writer.
		if copiedByteCounts.Out, err = copywithtimeout.Copy(opts.Stdout,
			processStdout,
			opts.ByteCounts.Out,
			fmt.Sprintf("CreateProcessEx: from stdout from %q", commandLine),
			time.Duration(4*time.Minute)); err != nil {
			return nil, nil, err
		}
	}

	// Copy the data back from stderr
	if opts.Stderr != nil {
		// Copy the data over to the writer.
		if copiedByteCounts.Err, err = copywithtimeout.Copy(opts.Stderr,
			processStderr,
			opts.ByteCounts.Err,
			fmt.Sprintf("CreateProcessEx: from stderr of %s", commandLine),
			time.Duration(4*time.Minute)); err != nil {
			return nil, nil, err
		}
	}

	logrus.Debugf("hcsshim: CreateProcessEx success: %q", commandLine)
	return proc, copiedByteCounts, nil
}
