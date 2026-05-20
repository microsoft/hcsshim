//go:build windows && functional

// V2 LCOW utility VM functional tests.
//
// These tests exercise the v2 vm.Controller lifecycle end-to-end against a
// real Hyper-V guest, validating every method on the v2 Controller interface
// that does not require a container or device sub-controller. They mirror the
// scope of the v1 TestLCOW_UVM_* tests, retargeted to the v2 builder + v2
// controller API:
//
//   v1                               | v2 equivalent here
//   ---------------------------------+-----------------------------------------
//   TestLCOW_UVM_Boot                | TestLCOW_V2_UVM_BootIterations
//   TestLCOW_UVM_KernelArgs          | TestLCOW_V2_UVM_KernelArgs
//   testuvm.CreateAndStartLCOW...    | TestLCOW_V2_UVM_Lifecycle
//   TestPropertiesGuestConnection... | TestLCOW_V2_UVM_Stats
//   (diagnostic helper)              | TestLCOW_V2_UVM_DumpStacks
//   (no v1 equivalent)               | TestLCOW_V2_UVM_ExecIntoHost
//   (no v1 equivalent)               | TestLCOW_V2_UVM_ConcurrentExec
//   (no v1 equivalent)               | TestLCOW_V2_UVM_StartIdempotent
//   (no v1 equivalent)               | TestLCOW_V2_UVM_TerminateIdempotent
//   (no v1 equivalent)               | TestLCOW_V2_UVM_TerminateFromCreated
//
// Container, device, and update tests are intentionally NOT covered here:
// vm.Controller exposes no container/device API and no Modify path; those
// live in cmd/containerd-shim-lcow-v2/service/ and are out of scope until
// internal/controller/container/ exists. Existing v1 LCOW container tests
// stay v1-only via the chokepoint in defaultLCOWOptions.
//
// VM lifecycle tests run serially — parallel Hyper-V VM creation strains
// nested-virt CI runners.

package functional

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	lcowbuilder "github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	hcscmd "github.com/Microsoft/hcsshim/internal/cmd"
	"github.com/Microsoft/hcsshim/internal/controller/vm"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/shimdiag"
	"github.com/Microsoft/hcsshim/osversion"
	vmspec "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

// buildLCOWV2Document resolves boot files (auto-detect first, with
// -linux-bootfiles flag override mirroring defaultLCOWOptions) and returns a
// minimal non-confidential v2 LCOW HCS ComputeSystem document plus the OCI
// bundle path used to build it. Tests skip if no boot files can be found.
func buildLCOWV2Document(t *testing.T) (*hcsschema.ComputeSystem, string) {
	t.Helper()

	bootFiles, err := testuvm.LCOWBootFilesPath()
	if err != nil {
		t.Logf("LCOWBootFilesPath: %v", err)
	}
	if p := *flagLinuxBootFilesPath; p != "" {
		bootFiles = p
	}
	if bootFiles == "" {
		t.Skip("no LCOW boot files found; set -linux-bootfiles or install ContainerPlat")
	}

	ctx := util.Context(context.Background(), t)
	shimOpts := &runhcsopts.Options{
		SandboxPlatform:   "linux/amd64",
		BootFilesRootPath: bootFiles,
	}
	bundle := t.TempDir()

	// Empty vmspec.Spec selects the non-confidential code path in BuildSandboxConfig.
	doc, _, err := lcowbuilder.BuildSandboxConfig(ctx, hcsOwner, bundle, shimOpts, &vmspec.Spec{})
	if err != nil {
		t.Fatalf("BuildSandboxConfig: %v", err)
	}
	return doc, bundle
}

// cleanupV2Controller registers a deferred TerminateVM+Wait with independent
// 30-second budgets. The compute-system sweep in TestMain is the ultimate
// backstop if both time out.
func cleanupV2Controller(t *testing.T, ctrl vm.Controller) {
	t.Helper()
	t.Cleanup(func() {
		termCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := ctrl.TerminateVM(termCtx); err != nil {
			t.Logf("TerminateVM failed (sweeper will reap): %v", err)
		}
		waitCtx, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel2()
		if err := ctrl.Wait(waitCtx); err != nil {
			t.Logf("Wait after Terminate failed: %v", err)
		}
	})
}

// createAndStartV2 is the common Create + Start dance used by every v2 test
// here. Returns the controller after StartVM succeeds; caller is responsible
// for cleanup (use cleanupV2Controller or explicit TerminateVM).
func createAndStartV2(t *testing.T, ctx context.Context) vm.Controller {
	t.Helper()
	doc, _ := buildLCOWV2Document(t)
	ctrl := vm.NewController()
	if err := ctrl.CreateVM(ctx, &vm.CreateOptions{ID: testName(t) + "@vm", HCSDocument: doc}); err != nil {
		t.Fatalf("CreateVM: %v", err)
	}
	if err := ctrl.StartVM(ctx, &vm.StartOptions{
		GCSServiceID: winio.VsockServiceID(prot.LinuxGcsVsockPort),
	}); err != nil {
		t.Fatalf("StartVM: %v", err)
	}
	return ctrl
}

// requireLCOWV2Uvm gates v2 controller tests on both the LCOWV2 feature flag
// and the uVM feature flag (nested-virt is mandatory for v2 controller tests).
func requireLCOWV2Uvm(t *testing.T) {
	t.Helper()
	require.Build(t, osversion.RS5)
	requireFeatures(t, featureLCOWV2, featureUVM)
}

// execInUVM runs args in the guest UVM with named-pipe stdio and returns
// (exitCode, stdout, stderr). The cmd package wires CreateStd{Out,Err}Pipe
// only when stdout/stderr paths are non-empty (see internal/cmd/cmd.go); we
// always provide pipes so processes that write to stdio don't get EPIPE.
func execInUVM(t *testing.T, ctx context.Context, ctrl vm.Controller, args []string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdoutPath, err := hcscmd.CreatePipeAndListen(&stdout, false)
	if err != nil {
		t.Fatalf("CreatePipeAndListen(stdout): %v", err)
	}
	stderrPath, err := hcscmd.CreatePipeAndListen(&stderr, false)
	if err != nil {
		t.Fatalf("CreatePipeAndListen(stderr): %v", err)
	}
	exitCode, err := ctrl.ExecIntoHost(ctx, &shimdiag.ExecProcessRequest{
		Args:   args,
		Stdout: stdoutPath,
		Stderr: stderrPath,
	})
	if err != nil {
		t.Fatalf("ExecIntoHost(%v): %v (stderr=%q)", args, err, stderr.String())
	}
	return exitCode, stdout.String(), stderr.String()
}

// TestLCOW_V2_UVM_Lifecycle validates the Create → Start state transitions of
// the v2 vm.Controller against a real Hyper-V guest.
func TestLCOW_V2_UVM_Lifecycle(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	if got := ctrl.State(); got != vm.StateRunning {
		t.Fatalf("expected StateRunning, got %s", got)
	}
	if ctrl.StartTime().IsZero() {
		t.Fatalf("expected non-zero StartTime")
	}
}

// TestLCOW_V2_UVM_BootIterations validates repeated boot+terminate cycles
// work — proves the v2 controller path can be re-used across multiple VM
// creations without leaking handles, mirroring the v1 TestLCOW_UVM_Boot
// iteration loop.
func TestLCOW_V2_UVM_BootIterations(t *testing.T) {
	requireLCOWV2Uvm(t)

	const iterations = 3
	for i := 0; i < iterations; i++ {
		ctx := util.Context(context.Background(), t)
		ctrl := createAndStartV2(t, ctx)
		if got := ctrl.State(); got != vm.StateRunning {
			t.Fatalf("iter %d: expected StateRunning, got %s", i, got)
		}
		termCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := ctrl.TerminateVM(termCtx)
		cancel()
		if err != nil {
			t.Fatalf("iter %d: TerminateVM: %v", i, err)
		}
		waitCtx, cancelW := context.WithTimeout(context.Background(), 30*time.Second)
		if werr := ctrl.Wait(waitCtx); werr != nil {
			t.Logf("iter %d: Wait after Terminate: %v", i, werr)
		}
		cancelW()
	}
}

// TestLCOW_V2_UVM_ExecIntoHost runs a command in the guest UVM via the GCS
// connection. Despite the method name, ExecIntoHost forwards to
// guest.ExecIntoUVM — execution happens inside the Linux guest rootfs, not on
// the Windows host.
func TestLCOW_V2_UVM_ExecIntoHost(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	const want = "hello"
	exitCode, stdout, stderr := execInUVM(t, ctx, ctrl, []string{"echo", want})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", exitCode, stderr)
	}
	if got := strings.TrimSpace(stdout); got != want {
		t.Fatalf("expected stdout %q, got %q (stderr=%q)", want, got, stderr)
	}
}

// TestLCOW_V2_UVM_KernelArgs reads /proc/cmdline from the guest and asserts
// the kernel args produced by the v2 builder match expectations — parity with
// the v1 TestLCOW_UVM_KernelArgs validation, retargeted to the v2 path.
func TestLCOW_V2_UVM_KernelArgs(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	exitCode, stdout, stderr := execInUVM(t, ctx, ctrl, []string{"cat", "/proc/cmdline"})
	if exitCode != 0 {
		t.Fatalf("cat /proc/cmdline exited %d (stderr=%q)", exitCode, stderr)
	}
	want := []string{
		"8250_core.nr_uarts=0",
		"panic=-1",
		"quiet",
		"pci=off",
		"init=/init",
	}
	for _, w := range want {
		if !strings.Contains(stdout, w) {
			t.Errorf("kernel cmdline missing %q (got: %s)", w, stdout)
		}
	}
}

// TestLCOW_V2_UVM_Stats queries memory + CPU statistics from the running VM.
// Validates the host-side stats collection path in vm.Controller.Stats.
func TestLCOW_V2_UVM_Stats(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	s, err := ctrl.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if s == nil {
		t.Fatalf("Stats returned nil response")
	}
	if s.Memory == nil {
		t.Fatalf("Stats.Memory is nil")
	}
}

// TestLCOW_V2_UVM_DumpStacks asks the guest to emit goroutine stacks via the
// GCS DumpStacks RPC. Validates the diagnostic path.
func TestLCOW_V2_UVM_DumpStacks(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	stacks, err := ctrl.DumpStacks(ctx)
	if err != nil {
		t.Fatalf("DumpStacks: %v", err)
	}
	// Empty stacks string is valid if the guest reports capability=false;
	// the call itself succeeding is the assertion. Log size for diagnostics.
	t.Logf("DumpStacks returned %d bytes", len(stacks))
}

// TestLCOW_V2_UVM_StartIdempotent validates that calling StartVM twice on the
// same controller is a no-op (matches the doc on Manager.StartVM). Containerd
// retries depend on this.
func TestLCOW_V2_UVM_StartIdempotent(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	if err := ctrl.StartVM(ctx, &vm.StartOptions{
		GCSServiceID: winio.VsockServiceID(prot.LinuxGcsVsockPort),
	}); err != nil {
		t.Fatalf("second StartVM (should be no-op): %v", err)
	}
	if got := ctrl.State(); got != vm.StateRunning {
		t.Fatalf("expected StateRunning after second StartVM, got %s", got)
	}
}

// TestLCOW_V2_UVM_TerminateIdempotent verifies that calling TerminateVM twice
// is safe. containerd retries StopPodSandbox / Delete, so the second call must
// be a no-op rather than an error.
func TestLCOW_V2_UVM_TerminateIdempotent(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	// No cleanupV2Controller — this test owns termination explicitly.

	if err := ctrl.TerminateVM(ctx); err != nil {
		t.Fatalf("first TerminateVM: %v", err)
	}
	if err := ctrl.TerminateVM(ctx); err != nil {
		t.Fatalf("second TerminateVM (should be no-op): %v", err)
	}
	if got := ctrl.State(); got != vm.StateTerminated {
		t.Fatalf("expected StateTerminated, got %s", got)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ctrl.Wait(waitCtx); err != nil {
		t.Logf("Wait after Terminate: %v", err)
	}
}

// TestLCOW_V2_UVM_TerminateFromCreated validates that TerminateVM works when
// called on a Created (not yet Started) controller — covers the failed-pod-
// create cleanup path where Start never ran.
func TestLCOW_V2_UVM_TerminateFromCreated(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	doc, _ := buildLCOWV2Document(t)
	ctrl := vm.NewController()

	if err := ctrl.CreateVM(ctx, &vm.CreateOptions{ID: testName(t) + "@vm", HCSDocument: doc}); err != nil {
		t.Fatalf("CreateVM: %v", err)
	}
	if got := ctrl.State(); got != vm.StateCreated {
		t.Fatalf("expected StateCreated, got %s", got)
	}
	if err := ctrl.TerminateVM(ctx); err != nil {
		t.Fatalf("TerminateVM from Created: %v", err)
	}
	if got := ctrl.State(); got != vm.StateTerminated {
		t.Fatalf("expected StateTerminated, got %s", got)
	}
}

// TestLCOW_V2_UVM_ConcurrentExec runs multiple ExecIntoHost calls concurrently
// to exercise activeExecCount and the GCS bridge under parallel load. All
// invocations should succeed with exit code 0.
func TestLCOW_V2_UVM_ConcurrentExec(t *testing.T) {
	requireLCOWV2Uvm(t)

	ctx := util.Context(context.Background(), t)
	ctrl := createAndStartV2(t, ctx)
	cleanupV2Controller(t, ctrl)

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ec, _, stderr := execInUVM(t, ctx, ctrl, []string{"echo", "concurrent"})
			if ec != 0 {
				errs[idx] = fmt.Errorf("concurrent exec idx=%d exit=%d stderr=%q", idx, ec, strings.TrimSpace(stderr))
			}
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			t.Errorf("%v", e)
		}
	}
}
