//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"errors"
	"testing"
	"unsafe"

	"github.com/containerd/containerd/v2/core/containers"
	ctrdoci "github.com/containerd/containerd/v2/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/Microsoft/hcsshim/osversion"

	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
)

// Test_Container_CPUAffinity_Argon is the CI-gating functional test for honoring
// spec.Windows.Resources.CPU.Affinity on process-isolated (Argon) WCOW containers
// (commit "hcsoci,hcs,shim: honor CPU affinity for Argon containers").
//
// It asserts the three layers from the validation strategy, all reachable from this
// one in-process test (the functional suite runs in-process with internal/jobobject
// and as SYSTEM, so it can open the silo job by name):
//
//	Layer 1 — the PR wrote the affinity to the silo's job object in the
//	          create→start window. This is the real regression gate: it fails if
//	          applyArgonCPUAffinity / SetSiloCPUGroupAffinities regresses.
//	Layer 2 — the host's view matches. The NT-variant silo job IS the host object,
//	          so the same GetCPUGroupAffinities read-back doubles as the host view;
//	          no second tool is needed.
//	Layer 3 — the init process is actually constrained. This is a kernel guarantee
//	          (the kernel propagates the silo job's affinity onto silo members), not
//	          hcsshim code. If the affinity cannot be read (OpenProcess /
//	          GetProcessGroupAffinity fail) the check is skipped, but a genuine
//	          mismatch is a hard failure: with Layer 1 passing, it points at the
//	          kernel/silo plumbing rather than this PR.
func Test_Container_CPUAffinity_Argon(t *testing.T) {
	requireFeatures(t, featureWCOW)
	// Affinity is applied via the silo job object on 20H2+ (the same floor as the
	// rest of the WCOW resource-update path).
	require.Build(t, osversion.V20H2)

	ctx := util.Context(namespacedContext(context.Background()), t)

	// Group 0 / single-mask works on any host, so it is the default CI case.
	t.Run("Group0SingleMask", func(t *testing.T) {
		want := []jobobject.GroupAffinity{{Group: 0, Mask: 0x3}} // CPUs 0 and 1.
		runArgonAffinityTest(ctx, t, want)
	})

	// A genuine multi-group pin needs a confirmed >1-processor-group host and
	// Windows Server 2022+; skip otherwise rather than assert against a topology
	// the runner does not have.
	t.Run("MultiGroup", func(t *testing.T) {
		require.Build(t, osversion.LTSC2022)
		if n := activeProcessorGroupCount(t); n < 2 {
			t.Skipf("multi-group affinity requires a host with >1 processor group, got %d", n)
		}
		want := []jobobject.GroupAffinity{
			{Group: 0, Mask: 0x1},
			{Group: 1, Mask: 0x1},
		}
		runArgonAffinityTest(ctx, t, want)
	})
}

// runArgonAffinityTest creates an Argon container pinned to want, then asserts the
// three validation layers.
func runArgonAffinityTest(ctx context.Context, t *testing.T, want []jobobject.GroupAffinity) {
	t.Helper()

	cID := testName(t, "container")
	scratch := testlayers.WCOWScratchDir(ctx, t, "")
	spec := testoci.CreateWindowsSpec(ctx, t, cID,
		testoci.DefaultWindowsSpecOpts(cID,
			ctrdoci.WithProcessCommandLine(testoci.PingSelfCmd),
			testoci.WithWindowsLayerFolders(append(windowsImageLayers(ctx, t), scratch)),
			withCPUAffinity(want),
		)...)

	// nil host => process-isolated (Argon). Create runs the PR's applyArgonCPUAffinity
	// between HCS-create and HCS-start.
	c, _, cleanup := testcontainer.Create(ctx, t, nil, spec, cID, hcsOwner)
	t.Cleanup(cleanup)

	// Layers 1 & 2, pre-start gate: the affinity is already recorded on the silo job
	// before the init process runs, proving "set after create, before start".
	assertSiloJobAffinity(ctx, t, cID, want)

	init := testcontainer.StartWithSpec(ctx, t, c, spec.Process, nil)
	t.Cleanup(func() {
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})

	// Layers 1 & 2 again, now that the silo has a running member.
	assertSiloJobAffinity(ctx, t, cID, want)

	// Layer 3 (kernel assertion): the init process inherited the pin. Skipped if the
	// affinity cannot be read; a real mismatch fails the test.
	assertProcessGroupAffinity(t, uint32(init.Process.Pid()), want)
}

// withCPUAffinity returns a SpecOpt that sets spec.Windows.Resources.CPU.Affinity.
func withCPUAffinity(affinities []jobobject.GroupAffinity) ctrdoci.SpecOpts {
	return func(_ context.Context, _ ctrdoci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Windows == nil {
			s.Windows = &specs.Windows{}
		}
		if s.Windows.Resources == nil {
			s.Windows.Resources = &specs.WindowsResources{}
		}
		if s.Windows.Resources.CPU == nil {
			s.Windows.Resources.CPU = &specs.WindowsCPUResources{}
		}
		oci := make([]specs.WindowsCPUGroupAffinity, len(affinities))
		for i, a := range affinities {
			oci[i] = specs.WindowsCPUGroupAffinity{Group: uint32(a.Group), Mask: a.Mask}
		}
		s.Windows.Resources.CPU.Affinity = oci
		return nil
	}
}

// assertSiloJobAffinity opens the container's server silo job object by its
// well-known name (\Container_<cID>) and asserts its CPU group affinities equal want.
// This is the host-side view of the object the PR wrote to (Layers 1 & 2).
func assertSiloJobAffinity(ctx context.Context, t *testing.T, cID string, want []jobobject.GroupAffinity) {
	t.Helper()

	job, err := jobobject.Open(ctx, &jobobject.Options{
		UseNTVariant: true,
		Name:         `\Container_` + cID,
	})
	if err != nil {
		t.Fatalf("open silo job for %q: %v", cID, err)
	}
	defer job.Close()

	got, err := job.GetCPUGroupAffinities()
	if err != nil {
		t.Fatalf("get silo job cpu group affinities: %v", err)
	}
	assertAffinitiesEqual(t, "silo job object", got, want)
}

// assertProcessGroupAffinity reads the group affinity the kernel placed on the init
// process and compares it to want. The PR only writes the job object; propagation
// onto silo members is a kernel guarantee. If the affinity cannot be read the check
// is skipped (logged, not failed), but a successful read that omits a pinned group
// is a hard failure.
func assertProcessGroupAffinity(t *testing.T, pid uint32, want []jobobject.GroupAffinity) {
	t.Helper()

	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		t.Logf("Layer 3 (kernel) skipped: OpenProcess(%d): %v", pid, err)
		return
	}
	defer windows.CloseHandle(h) //nolint:errcheck

	got, err := getProcessGroupAffinity(h)
	if err != nil {
		t.Logf("Layer 3 (kernel) skipped: GetProcessGroupAffinity(%d): %v", pid, err)
		return
	}

	// The process reports the set of groups it may run on; assert every group we
	// pinned shows up. We do not compare masks here: the kernel reports the group's
	// active-processor mask for the process, not necessarily the bits we requested.
	wantGroups := make(map[uint16]struct{}, len(want))
	for _, a := range want {
		wantGroups[a.Group] = struct{}{}
	}
	gotGroups := make(map[uint16]struct{}, len(got))
	for _, g := range got {
		gotGroups[g] = struct{}{}
	}
	for g := range wantGroups {
		if _, ok := gotGroups[g]; !ok {
			t.Errorf("Layer 3 (kernel): init process not constrained to group %d; process groups = %v", g, got)
		}
	}
}

func assertAffinitiesEqual(t *testing.T, what string, got, want []jobobject.GroupAffinity) {
	t.Helper()

	// Order-independent compare keyed by group: the OS does not promise to return
	// entries in the order they were set.
	if len(got) != len(want) {
		t.Fatalf("%s affinity: got %+v, want %+v (length mismatch)", what, got, want)
	}
	byGroup := make(map[uint16]uint64, len(got))
	for _, g := range got {
		byGroup[g.Group] = g.Mask
	}
	for _, w := range want {
		mask, ok := byGroup[w.Group]
		if !ok {
			t.Fatalf("%s affinity: missing group %d; got %+v, want %+v", what, w.Group, got, want)
		}
		if mask != w.Mask {
			t.Fatalf("%s affinity: group %d mask = %#x, want %#x", what, w.Group, mask, w.Mask)
		}
	}
}

var (
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessGroupAffinity    = kernel32.NewProc("GetProcessGroupAffinity")
	procGetActiveProcessorGroupCnt = kernel32.NewProc("GetActiveProcessorGroupCount")
)

// getProcessGroupAffinity wraps kernel32!GetProcessGroupAffinity, which is not bound
// in golang.org/x/sys/windows. It returns the processor groups the process may run on.
func getProcessGroupAffinity(h windows.Handle) ([]uint16, error) {
	// Probe with a small buffer; the call sets count to the required size and fails
	// with ERROR_INSUFFICIENT_BUFFER if it is too small.
	groups := make([]uint16, 4)
	count := uint16(len(groups))
	for {
		r1, _, e := procGetProcessGroupAffinity.Call(
			uintptr(h),
			uintptr(unsafe.Pointer(&count)),
			uintptr(unsafe.Pointer(&groups[0])),
		)
		if r1 != 0 {
			return groups[:count], nil
		}
		if errors.Is(e, windows.ERROR_INSUFFICIENT_BUFFER) && int(count) > len(groups) {
			groups = make([]uint16, count)
			continue
		}
		return nil, e
	}
}

// activeProcessorGroupCount returns the number of active processor groups on the host,
// used to decide whether a multi-group affinity test can run.
func activeProcessorGroupCount(t *testing.T) int {
	t.Helper()
	r1, _, _ := procGetActiveProcessorGroupCnt.Call()
	return int(uint16(r1))
}
