//go:build windows

package uvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"

	"github.com/Microsoft/hcsshim/test/internal/sync"
)

// since the tests can run from other directories, lookup other directories first.
// return empty string to rely on the defaults set by [uvm.NewDefaultOptionsLCOW]
var lcowOSBootFiles = sync.NewLazyString(func() (string, error) {
	if p, err := exec.LookPath("containerd.exe"); err != nil {
		return filepath.Join(p, "LinuxBootFiles"), nil
	}
	p := `C:\ContainerPlat\LinuxBootFiles`
	if _, err := os.Stat(p); err == nil {
		return p, nil
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
func DefaultLCOWOptions(tb testing.TB, id, owner string) *uvm.OptionsLCOW {
	tb.Helper()
	opts := uvm.NewDefaultOptionsLCOW(id, owner)
	if p := lcowOSBootFiles.String(tb); p != "" {
		tb.Logf("using LCOW bootfiles path: %s", p)
		opts.BootFilesPath = p
	}
	return opts
}

// CreateAndStartLCOW with all default options.
func CreateAndStartLCOW(ctx context.Context, tb testing.TB, id string) *uvm.UtilityVM {
	tb.Helper()
	return CreateAndStartLCOWFromOpts(ctx, tb, DefaultLCOWOptions(tb, id, ""))
}

// CreateAndStartLCOWFromOpts creates an LCOW utility VM with the specified options.
func CreateAndStartLCOWFromOpts(ctx context.Context, tb testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	tb.Helper()

	if opts == nil {
		tb.Fatal("opts must be set")
	}

	vm := CreateLCOW(ctx, tb, opts)
	cleanup := Start(ctx, tb, vm)
	tb.Cleanup(cleanup)

	return vm
}

func CreateLCOW(ctx context.Context, tb testing.TB, opts *uvm.OptionsLCOW) *uvm.UtilityVM {
	tb.Helper()
	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		tb.Fatalf("could not create LCOW UVM: %v", err)
	}

	return vm
}

func SetSecurityPolicy(ctx context.Context, tb testing.TB, vm *uvm.UtilityVM, policy string) {
	tb.Helper()
	if err := vm.SetConfidentialUVMOptions(ctx, uvm.WithSecurityPolicy(policy)); err != nil {
		tb.Fatalf("could not set vm security policy to %q: %v", policy, err)
	}
}
