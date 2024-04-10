//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/osversion"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// TODO: vSMB benchmarks
// TODO: re-add a removed directmapped vSMB share
// TODO: add vSMB to created-but-not-started (or closed) uVM

// TestVSMB_WCOW tests adding/removing VSMB layers from a v2 Windows utility VM.
func TestVSMB_WCOW(t *testing.T) {
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureWCOW, featureUVM, featureVSMB)

	ctx := util.Context(namespacedContext(context.Background()), t)

	type testCase struct {
		name        string
		backupPriv  bool
		readOnly    bool
		noDirectMap bool
	}
	tests := make([]testCase, 0, 8)
	for _, ro := range []bool{true, false} {
		for _, backup := range []bool{true, false} {
			for _, noDirectMap := range []bool{true, false} {
				n := "RW"
				if ro {
					n = "RO"
				}
				if backup {
					n += "-backup"
				}
				if noDirectMap {
					n += "-noDirectMap"
				}

				tests = append(tests, testCase{
					name:        n,
					backupPriv:  backup,
					readOnly:    ro,
					noDirectMap: noDirectMap,
				})
			}
		}
	}

	const iterations = 64
	for _, tt := range tests {
		for _, newDir := range []bool{true, false} {
			name := tt.name
			if newDir {
				name += "-newDir"
			}

			t.Run("dir-"+name, func(t *testing.T) {
				// create a temp directory before creating the uVM, so the uVM will be closed before
				// temp dir's cleanup
				dir := t.TempDir()
				vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

				options := vm.DefaultVSMBOptions(tt.readOnly)
				options.TakeBackupPrivilege = tt.backupPriv
				options.NoDirectmap = tt.noDirectMap
				t.Logf("vSMB options: %#+v", options)

				var path string
				var err error
				for i := 0; i < iterations; i++ {
					if i == 0 || newDir {
						// create a temp directory on the first iteration, or on each subsequent iteration if [testCase.newDir]
						// don't need to remove it, since `dir` will be removed whole-sale during test cleanup
						if path, err = os.MkdirTemp(dir, ""); err != nil {
							t.Fatalf("MkdirTemp: %v", err)
						}
					}

					opts := *options // create a copy in case its (accidentally) modified
					s := testuvm.AddVSMB(ctx, t, vm, path, &opts)
					if path != s.HostPath {
						t.Fatalf("expected vSMB path: %q; got %q", path, s.HostPath)
					}
				}
			})

			t.Run("file-"+name, func(t *testing.T) {
				// create a temp directory before creating the uVM, so the uVM will be closed before
				// temp dir's cleanup
				dir := t.TempDir()
				vm := testuvm.CreateAndStart(ctx, t, defaultWCOWOptions(ctx, t))

				options := vm.DefaultVSMBOptions(tt.readOnly)
				options.TakeBackupPrivilege = tt.backupPriv
				options.NoDirectmap = tt.noDirectMap
				t.Logf("vSMB options: %#+v", options)

				var path string
				var err error
				for i := 0; i < iterations; i++ {
					if i == 0 || newDir {
						// create a temp directory on the first iteration, or on each subsequent iteration if [testCase.newDir]
						// don't need to remove it, since `dir` will be removed whole-sale during test cleanup
						if path, err = os.MkdirTemp(dir, ""); err != nil {
							t.Fatalf("MkdirTemp: %v", err)
						}
					}
					f := filepath.Join(path, fmt.Sprintf("f%d.txt", i))
					if err := os.WriteFile(f, []byte(t.Name()), 0600); err != nil {
						t.Fatal(err)
					}

					opts := *options // create a copy in case its (accidentally) modified
					s := testuvm.AddVSMB(ctx, t, vm, f, &opts)
					if path != s.HostPath {
						t.Fatalf("expected vSMB path: %q; got %q", path, s.HostPath)
					}
				}
			})
		}
	}

	t.Run("NoWritableFileShares", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// create a temp directory before creating the uVM, so the uVM will be closed before
				// temp dir's cleanup
				dir := t.TempDir()

				opts := defaultWCOWOptions(ctx, t)
				opts.NoWritableFileShares = true
				vm := testuvm.CreateAndStart(ctx, t, opts)

				options := vm.DefaultVSMBOptions(tt.readOnly)
				options.TakeBackupPrivilege = tt.backupPriv
				options.NoDirectmap = tt.noDirectMap
				t.Logf("vSMB options: %#+v", options)

				s, err := vm.AddVSMB(ctx, dir, options)

				t.Cleanup(func() {
					if err != nil {
						return
					}
					if err = vm.RemoveVSMB(ctx, s.HostPath, tt.readOnly); err != nil {
						t.Fatalf("failed to remove vSMB share: %v", err)
					}
				})

				if !tt.readOnly && !errors.Is(err, hcs.ErrOperationDenied) {
					t.Fatalf("AddVSMB should have failed with %v instead of: %v", hcs.ErrOperationDenied, err)
				}
			})
		}
	})
}
