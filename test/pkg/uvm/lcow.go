//go:build windows

package uvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/sync"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

var lcowOSBootFilesOnce = sync.OnceValue(func() (string, error) {
	// since the tests can be run from directories outside of where containerd and the
	// LinuxBootFiles are, search through potential locations for the boot files
	// first start with where containerd is, since there may be a leftover C:\ContainerPlat
	// directory from a prior install.
	paths := make([]string, 0, 2)
	if p, err := exec.LookPath("containerd.exe"); err != nil {
		paths = append(paths, p)
	}
	paths = append(paths, `C:\ContainerPlat`)
	for _, p := range paths {
		p = filepath.Join(p, "LinuxBootFiles")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", nil
})

// DefaultLCOWOptions returns default options for a bootable LCOW uVM, but first checks
// if `containerd.exe` is in the path, or C:\ContainerPlat\LinuxBootFiles exists, and
// prefers those paths above the default boot path set by [uvm.NewDefaultOptionsLCOW]
//
// This is accounts for the tests being run a location different from the containerd.exe
// path.
//
// See [uvm.NewDefaultOptionsLCOW] for more information.
func DefaultLCOWOptions(ctx context.Context, tb testing.TB, id, owner string) *uvm.OptionsLCOW {
	tb.Helper()

	opts := uvm.NewDefaultOptionsLCOW(id, owner)
	if v, _ := lcowOSBootFilesOnce(); v != "" {
		opts.UpdateBootFilesPath(ctx, v)
	}
	return opts
}

// CreateAndStartLCOW with all default options.
//
// See [CreateAndStartLCOWFromOpts].
func CreateAndStartLCOW(ctx context.Context, tb testing.TB, id string) *uvm.UtilityVM {
	tb.Helper()
	return CreateAndStartLCOWFromOpts(ctx, tb, DefaultLCOWOptions(ctx, tb, id, ""))
}

// CreateAndStartLCOWFromOpts creates an LCOW utility VM with the specified options.
//
// The cleanup function will be added to `tb.Cleanup`.
func CreateAndStartLCOWFromOpts(ctx context.Context, tb testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	tb.Helper()

	if opts == nil {
		tb.Fatal("opts must be set")
	}

	vm, cleanup := CreateLCOW(ctx, tb, opts)
	tb.Cleanup(func() { cleanup(ctx) })
	Start(ctx, tb, vm)

	return vm
}

func CreateLCOW(ctx context.Context, tb testing.TB, opts *uvm.OptionsLCOW) (*uvm.UtilityVM, CleanupFn) {
	tb.Helper()
	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		tb.Fatalf("could not create LCOW UVM: %v", err)
	}

	return vm, newCleanupFn(ctx, tb, vm)
}

func SetSecurityPolicy(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, policy string) {
	tb.Helper()
	if err := vm.SetConfidentialUVMOptions(ctx, uvm.WithSecurityPolicy(policy)); err != nil {
		tb.Fatalf("could not set vm security policy to %q: %v", policy, err)
	}
}
