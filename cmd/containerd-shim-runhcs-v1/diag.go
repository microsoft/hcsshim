package main

import (
	"context"
	"io"

	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

type taskDiagnostics interface {
	ExecInHost(ctx context.Context, req *shimdiag.ExecProcessRequest) (int, error)
}

func execInUvm(ctx context.Context, vm *uvm.UtilityVM, req *shimdiag.ExecProcessRequest) (int, error) {
	np, err := newNpipeIO(ctx, "", "", req.Stdin, req.Stdout, req.Stderr, req.Terminal)
	if err != nil {
		return 0, err
	}
	defer np.Close()
	wd := req.Workdir
	if wd == "" {
		if vm.OS() == "windows" {
			wd = `c:\`
		} else {
			wd = "/"
		}
	}
	proc, err := vm.ComputeSystem().CreateProcess(&hcsschema.ProcessParameters{
		CommandArgs:      req.Args,
		CreateStdInPipe:  req.Stdin != "",
		CreateStdOutPipe: req.Stdout != "",
		CreateStdErrPipe: req.Stderr != "",
		EmulateConsole:   req.Terminal,
		WorkingDirectory: wd,
	})
	if err != nil {
		return 0, err
	}
	pin, pout, perr := proc.Stdio()
	if err != nil {
		return 0, err
	}
	if pin != nil {
		go func() {
			io.Copy(pin, np.Stdin())
			proc.CloseStdin()
		}()
	}
	if pout != nil {
		go func() {
			io.Copy(np.Stdout(), pout)
		}()
	}
	if perr != nil {
		go func() {
			io.Copy(np.Stderr(), perr)
		}()
	}
	err = proc.Wait()
	if err != nil {
		proc.Kill()
		proc.Wait()
	}
	ec, err := proc.ExitCode()
	proc.Close()
	return ec, err
}
