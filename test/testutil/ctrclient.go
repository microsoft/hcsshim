package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func DefaultCtrPath() string {
	return filepath.Join(filepath.Dir(os.Args[0]), "ctr.exe")
}

// Global options for connecting to ctr.exe
// Flags are passed to parent module, cannot import them here without causing circular dependencies
// todo: restructure `LayerFolders` and `CreateWCOWUVM*` functions to use CtrClientOptions
// or move `utilities/*` into parent path, similar to `tests/cri-containerd`

type CtrClientOptions struct {
	Ctrd CtrdClientOptions
	Path string
}

func (co CtrClientOptions) Command(ctx context.Context, arg ...string) *exec.Cmd {
	args := []string{
		"--address",
		co.Ctrd.Address,
		"--namespace",
		co.Ctrd.Namespace,
	}
	args = append(args, arg...)
	cmd := exec.CommandContext(ctx, co.Path, args...)
	return cmd
}

func (co CtrClientOptions) PullImage(ctx context.Context, t *testing.T, snapshotter, image string) {
	cmd := co.Command(ctx, "images",
		"pull",
		"--snapshotter",
		snapshotter,
		"view",
		image)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to pull image %q with %v. Command was %v", image, err, cmd)
	}
}
