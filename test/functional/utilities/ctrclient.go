package testutilities

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func DefaultCtrPath() string {
	return filepath.Join(filepath.Dir(os.Args[0]), "ctr.exe")
}

// Global options for connecting to ctr.exe
// Flags are passed to parent module, cannot import them here without causing circular dependencies
// todo: restructure `LayerFolders` and `CreateWCOWUVM*` functions to use CtrClientOptions
// or move `utilities/*` into parent path, similar to `tests/cri-containerd`

type CtrClientOptions struct {
	CtrdClientOptions
	Path      string
}

var ctro CtrClientOptions

func GetCtrClientOptions() *CtrClientOptions {
	return &ctro
}

func (co CtrClientOptions) Command(ctx context.Context,arg ...string) *exec.Cmd {
	args := []string{
		"--address",
		co.Address,
		"--namespace",
		co.Namespace,
	}
	args = append(args, arg...)
	cmd := exec.CommandContext(ctx, co.Path, args...)
	return cmd
}

func (co CtrClientOptions) PullImage(ctx context.Context, snapshotter, image string) error {
	cmd := co.Command(ctx, "images",
		"pull",
		"--snapshotter",
		snapshotter,
		"view",
		image)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Failed to pull image %q with %v. Command was %v", image, err, cmd)
	}
	return nil
}

func CtrCommand(arg ...string) *exec.Cmd {
	return ctro.Command(context.Background(), arg...)
}
