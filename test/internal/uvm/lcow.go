//go:build windows

package uvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/uvm"
)

var lcowOSBootFiles string

func init() {
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
			lcowOSBootFiles = p
			break
		}
	}
}

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
	if lcowOSBootFiles != "" {
		tb.Logf("using LCOW bootfiles path: %s", lcowOSBootFiles)
		opts.BootFilesPath = lcowOSBootFiles
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
