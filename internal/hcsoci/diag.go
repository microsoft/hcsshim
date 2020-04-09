package hcsoci

import (
	"context"
	"errors"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

func ExecInUvm(ctx context.Context, vm *uvm.UtilityVM, req *shimdiag.ExecProcessRequest) (int, error) {
	if len(req.Args) == 0 {
		return 0, errors.New("missing command")
	}
	np, err := NewNpipeIO(ctx, req.Stdin, req.Stdout, req.Stderr, req.Terminal)
	if err != nil {
		return 0, err
	}
	defer np.Close(ctx)
	cmd := CommandContext(ctx, vm, req.Args[0], req.Args[1:]...)
	if req.Workdir != "" {
		cmd.Spec.Cwd = req.Workdir
	}
	if vm.OS() == "windows" {
		cmd.Spec.User.Username = `NT AUTHORITY\SYSTEM`
	}
	cmd.Spec.Terminal = req.Terminal
	cmd.Stdin = np.Stdin()
	cmd.Stdout = np.Stdout()
	cmd.Stderr = np.Stderr()
	cmd.Log = log.G(ctx).WithField(logfields.UVMID, vm.ID())
	err = cmd.Run()
	return cmd.ExitState.ExitCode(), err
}
