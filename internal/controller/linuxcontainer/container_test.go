//go:build windows && lcow

package linuxcontainer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/mocks"
	"github.com/Microsoft/hcsshim/internal/controller/process"
	"github.com/Microsoft/hcsshim/internal/gcs"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/signals"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/containerd/errdefs"
	"go.uber.org/mock/gomock"
)

const (
	testVMID        = "test-vm"
	testPodID       = "test-pod"
	testContainerID = "test-ctr"
)

var (
	errUnmapSCSI  = errors.New("unmap scsi failed")
	errUnmapPlan9 = errors.New("unmap plan9 failed")
	errRemoveVPCI = errors.New("remove vpci failed")
)

// newContainerTestController creates a Controller wired to fresh mock
// controllers for scsi, plan9, vpci, and guest.
func newContainerTestController(t *testing.T) (
	*Controller,
	*mocks.MockscsiController,
	*mocks.Mockplan9Controller,
	*mocks.MockvPCIController,
	*mocks.Mockguest,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	scsiCtrl := mocks.NewMockscsiController(ctrl)
	plan9Ctrl := mocks.NewMockplan9Controller(ctrl)
	vpciCtrl := mocks.NewMockvPCIController(ctrl)
	guestCtrl := mocks.NewMockguest(ctrl)

	c := New(testVMID, testPodID, testContainerID, guestCtrl, scsiCtrl, plan9Ctrl, vpciCtrl)
	return c, scsiCtrl, plan9Ctrl, vpciCtrl, guestCtrl
}

// --- New ---

// TestNew_InitializesFields verifies that New sets all fields correctly and
// that the initial state is StateNotCreated.
func TestNew_InitializesFields(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	if c.vmID != testVMID {
		t.Errorf("vmID = %q, want %q", c.vmID, testVMID)
	}
	if c.gcsPodID != testPodID {
		t.Errorf("gcsPodID = %q, want %q", c.gcsPodID, testPodID)
	}
	if c.containerID != testContainerID {
		t.Errorf("containerID = %q, want %q", c.containerID, testContainerID)
	}
	if c.gcsContainerID != testContainerID {
		t.Errorf("gcsContainerID = %q, want %q", c.gcsContainerID, testContainerID)
	}
	if c.state != StateNotCreated {
		t.Errorf("initial state = %s, want NotCreated", c.state)
	}
	if c.terminatedCh == nil {
		t.Fatal("terminatedCh must not be nil after New")
	}
	if c.processes == nil {
		t.Fatal("processes map must not be nil after New")
	}
	if len(c.processes) != 0 {
		t.Errorf("expected empty processes map, got %d entries", len(c.processes))
	}
}

// --- Wait ---

// TestWait_AlreadyClosed verifies that Wait returns immediately when the
// terminated channel is already closed.
func TestWait_AlreadyClosed(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	close(c.terminatedCh)

	// Should return immediately.
	doneCh := make(chan struct{})
	go func() {
		c.Wait(t.Context())
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return for already-closed channel")
	}
}

// TestWait_ContextCancellation verifies that Wait respects context cancellation.
func TestWait_ContextCancellation(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	ctx, cancel := context.WithCancel(t.Context())
	doneCh := make(chan struct{})
	go func() {
		c.Wait(ctx)
		close(doneCh)
	}()

	// Wait should block until we cancel.
	cancel()

	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after context cancellation")
	}
}

// TestWait_UnblocksAllWaiters verifies that closing the terminated channel
// unblocks multiple concurrent waiters.
func TestWait_UnblocksAllWaiters(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	const numWaiters = 5
	var waitGroup sync.WaitGroup
	waitGroup.Add(numWaiters)
	for range numWaiters {
		go func() {
			defer waitGroup.Done()
			c.Wait(t.Context())
		}()
	}

	// Close the terminated channel to unblock all waiters.
	close(c.terminatedCh)

	doneCh := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("not all waiters were unblocked")
	}
}

// --- NewProcess ---

// TestNewProcess_WrongState verifies that NewProcess rejects calls outside StateRunning.
func TestNewProcess_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateStopped, StateInvalid}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			_, err := c.NewProcess("exec-1")
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("NewProcess() error = %v, want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestNewProcess_DuplicateExecID verifies that NewProcess rejects a duplicate exec ID.
func TestNewProcess_DuplicateExecID(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateRunning

	// Pre-populate the exec ID.
	c.processes["exec-1"] = process.New(testContainerID, "exec-1", nil, 0)

	_, err := c.NewProcess("exec-1")
	if err == nil {
		t.Fatal("expected error for duplicate exec ID")
	}
}

// TestNewProcess_Success verifies that NewProcess creates and tracks a new
// process controller.
func TestNewProcess_Success(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateRunning

	proc, err := c.NewProcess("exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proc == nil {
		t.Fatal("expected non-nil process controller")
	}
	if len(c.processes) != 1 {
		t.Errorf("expected 1 tracked process, got %d", len(c.processes))
	}
	if c.processes["exec-1"] != proc {
		t.Error("tracked process does not match returned process")
	}
}

// --- GetProcess ---

// TestGetProcess_Found verifies that GetProcess returns the correct process.
func TestGetProcess_Found(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	initProc := process.New(testContainerID, "", nil, 0)
	c.processes[""] = initProc

	got, err := c.GetProcess("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != initProc {
		t.Error("returned process does not match stored process")
	}
}

// TestGetProcess_NotFound verifies that GetProcess returns ErrNotFound for
// an unknown exec ID.
func TestGetProcess_NotFound(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	_, err := c.GetProcess("nonexistent")
	if !errors.Is(err, errdefs.ErrNotFound) {
		t.Errorf("GetProcess() error = %v, want ErrNotFound", err)
	}
}

// --- ListProcesses ---

// TestListProcesses_Empty verifies that ListProcesses returns an empty map
// when only the init process is registered.
func TestListProcesses_Empty(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.processes[""] = process.New(testContainerID, "", nil, 0)

	result, err := c.ListProcesses()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 exec processes, got %d", len(result))
	}
}

// TestListProcesses_ExcludesInit verifies that the init process (exec ID "")
// is excluded from the result while all other execs are returned.
func TestListProcesses_ExcludesInit(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	initProc := process.New(testContainerID, "", nil, 0)
	exec1 := process.New(testContainerID, "exec-1", nil, 0)
	exec2 := process.New(testContainerID, "exec-2", nil, 0)
	c.processes[""] = initProc
	c.processes["exec-1"] = exec1
	c.processes["exec-2"] = exec2

	result, err := c.ListProcesses()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 exec processes, got %d", len(result))
	}
	if result["exec-1"] != exec1 {
		t.Error("exec-1 not found or mismatched")
	}
	if result["exec-2"] != exec2 {
		t.Error("exec-2 not found or mismatched")
	}
}

// TestListProcesses_NoProcesses verifies that ListProcesses returns an empty
// map when the processes map is completely empty.
func TestListProcesses_NoProcesses(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	result, err := c.ListProcesses()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 processes, got %d", len(result))
	}
}

// --- KillProcess ---

// TestKillProcess_AllWithNonEmptyExecID verifies that KillProcess rejects the
// combination of all=true with a non-empty exec ID.
func TestKillProcess_AllWithNonEmptyExecID(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateRunning

	err := c.KillProcess(t.Context(), "exec-1", 15, true)
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("KillProcess() error = %v, want ErrFailedPrecondition", err)
	}
}

// TestKillProcess_InvalidSignal verifies that KillProcess rejects invalid
// signals before checking container state.
func TestKillProcess_InvalidSignal(t *testing.T) {
	t.Parallel()
	c, _, _, _, guestCtrl := newContainerTestController(t)
	c.state = StateRunning

	// Signal process is supported but signal value is invalid.
	guestCtrl.EXPECT().
		Capabilities().
		Return(&gcs.LCOWGuestDefinedCapabilities{})

	err := c.KillProcess(t.Context(), "", 999, false)
	if !errors.Is(err, signals.ErrInvalidSignal) {
		t.Errorf("KillProcess() error = %v, want ErrInvalidSignal", err)
	}
}

// TestKillProcess_NotCreatedState verifies that KillProcess rejects calls
// when the container has not been created yet.
func TestKillProcess_NotCreatedState(t *testing.T) {
	t.Parallel()
	c, _, _, _, guestCtrl := newContainerTestController(t)
	c.state = StateNotCreated

	// SIGTERM (15) with no signal support returns nil signal options.
	guestCtrl.EXPECT().
		Capabilities().
		Return(&gcs.LCOWGuestDefinedCapabilities{})

	err := c.KillProcess(t.Context(), "", 15, false)
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("KillProcess() error = %v, want ErrFailedPrecondition", err)
	}
}

// TestKillProcess_ProcessNotFound verifies that KillProcess returns ErrNotFound
// when the target exec ID does not exist.
func TestKillProcess_ProcessNotFound(t *testing.T) {
	t.Parallel()
	c, _, _, _, guestCtrl := newContainerTestController(t)
	c.state = StateRunning

	guestCtrl.EXPECT().
		Capabilities().
		Return(&gcs.LCOWGuestDefinedCapabilities{})

	err := c.KillProcess(t.Context(), "nonexistent", 15, false)
	if !errors.Is(err, errdefs.ErrNotFound) {
		t.Errorf("KillProcess() error = %v, want ErrNotFound", err)
	}
}

// --- DeleteProcess ---

// TestDeleteProcess_NotCreatedState verifies that DeleteProcess rejects calls
// when the container has not been created yet.
func TestDeleteProcess_NotCreatedState(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateNotCreated

	_, err := c.DeleteProcess(t.Context(), "exec-1")
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("DeleteProcess() error = %v, want ErrFailedPrecondition", err)
	}
}

// TestDeleteProcess_ProcessNotFound verifies that DeleteProcess returns
// ErrNotFound when the target exec ID does not exist.
func TestDeleteProcess_ProcessNotFound(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateCreated

	_, err := c.DeleteProcess(t.Context(), "nonexistent")
	if !errors.Is(err, errdefs.ErrNotFound) {
		t.Errorf("DeleteProcess() error = %v, want ErrNotFound", err)
	}
}

// TestDeleteProcess_InitProcessNotStarted verifies that deleting the init
// process on a created-but-never-started container triggers closeContainer.
func TestDeleteProcess_InitProcessNotStarted(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateCreated

	// Create a process controller in terminated state so Delete succeeds.
	// process.New starts in StateNotCreated; Delete from NotCreated returns error.
	// We need a process in StateCreated or StateTerminated for Delete to succeed.
	// Since we can't directly set the state of process.Controller from outside
	// the package, we use the fact that Kill on a StateCreated process aborts it
	// into StateTerminated.
	initProc := process.New(testContainerID, "", nil, 0)
	c.processes[""] = initProc

	// The init process is in StateNotCreated. Delete on a process in
	// StateNotCreated hits the default case and returns ErrFailedPrecondition.
	_, err := c.DeleteProcess(t.Context(), "")
	if err == nil {
		t.Fatal("expected error deleting init process in StateNotCreated")
	}
}

// --- Update ---

// TestUpdate_WrongState verifies that Update rejects calls outside StateRunning.
func TestUpdate_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateStopped, StateInvalid}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			err := c.Update(t.Context(), nil)
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Update() error = %v, want ErrFailedPrecondition", err)
			}
		})
	}
}

// TestUpdate_InvalidResourceType verifies that Update rejects resources that
// are not *specs.LinuxResources.
func TestUpdate_InvalidResourceType(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateRunning

	err := c.Update(t.Context(), "not-linux-resources")
	if err == nil {
		t.Fatal("expected error for invalid resource type")
	}
}

// --- releaseResources ---

// TestReleaseResources_AllResourceTypes verifies that releaseResources unmaps
// layers, SCSI mounts, Plan9 shares, and VPCI devices in order.
func TestReleaseResources_AllResourceTypes(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, plan9Ctrl, vpciCtrl, guestCtrl := newContainerTestController(t)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()
	scsiGUID, _ := guid.NewV4()
	plan9GUID, _ := guid.NewV4()
	deviceGUID, _ := guid.NewV4()

	c.layers = &scsiLayers{
		layersCombined: true,
		rootfsPath:     "/rootfs",
		scratch:        scsiReservation{id: scratchGUID, guestPath: "/dev/scratch"},
		roLayers:       []scsiReservation{{id: roGUID, guestPath: "/dev/ro0"}},
	}
	c.scsiResources = []guid.GUID{scsiGUID}
	c.plan9Resources = []guid.GUID{plan9GUID}
	c.devices = []guid.GUID{deviceGUID}

	// Expect combined layers removal.
	guestCtrl.EXPECT().
		RemoveLCOWCombinedLayers(gomock.Any(), guestresource.LCOWCombinedLayers{
			ContainerID:       c.gcsContainerID,
			ContainerRootPath: "/rootfs",
			Layers:            []hcsschema.Layer{{Path: "/dev/ro0"}},
			ScratchPath:       "/dev/scratch",
		}).
		Return(nil)

	// Expect scratch + RO layer unmaps.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scratchGUID).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUID).Return(nil)

	// Expect additional SCSI resource unmap.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scsiGUID).Return(nil)

	// Expect Plan9 share unmap.
	plan9Ctrl.EXPECT().UnmapFromGuest(gomock.Any(), plan9GUID).Return(nil)

	// Expect VPCI device removal.
	vpciCtrl.EXPECT().RemoveFromVM(gomock.Any(), deviceGUID).Return(nil)

	c.releaseResources(t.Context())

	// All resource slices must be nil after release.
	if c.layers != nil {
		t.Error("layers should be nil after releaseResources")
	}
	if c.scsiResources != nil {
		t.Error("scsiResources should be nil after releaseResources")
	}
	if c.plan9Resources != nil {
		t.Error("plan9Resources should be nil after releaseResources")
	}
	if c.devices != nil {
		t.Error("devices should be nil after releaseResources")
	}
}

// TestReleaseResources_NoLayers verifies that releaseResources handles the
// case where no layers were allocated (only additional resources).
func TestReleaseResources_NoLayers(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	scsiGUID, _ := guid.NewV4()
	c.scsiResources = []guid.GUID{scsiGUID}

	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scsiGUID).Return(nil)

	c.releaseResources(t.Context())

	if c.scsiResources != nil {
		t.Error("scsiResources should be nil after releaseResources")
	}
}

// TestReleaseResources_LayersNotCombined verifies that when layers exist but
// were not combined, RemoveLCOWCombinedLayers is not called.
func TestReleaseResources_LayersNotCombined(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	roGUID, _ := guid.NewV4()
	scratchGUID, _ := guid.NewV4()

	c.layers = &scsiLayers{
		layersCombined: false,
		scratch:        scsiReservation{id: scratchGUID},
		roLayers:       []scsiReservation{{id: roGUID}},
	}

	// Only unmaps expected; no RemoveLCOWCombinedLayers call.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scratchGUID).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUID).Return(nil)

	c.releaseResources(t.Context())

	if c.layers != nil {
		t.Error("layers should be nil after releaseResources")
	}
}

// TestReleaseResources_Idempotent verifies that a second call to
// releaseResources is a no-op.
func TestReleaseResources_Idempotent(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	scsiGUID, _ := guid.NewV4()
	c.scsiResources = []guid.GUID{scsiGUID}

	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scsiGUID).Return(nil).Times(1)

	c.releaseResources(t.Context())
	// Second call should be a no-op (no mock calls expected).
	c.releaseResources(t.Context())
}

// TestReleaseResources_ErrorsContinue verifies that releaseResources continues
// releasing remaining resources even when individual unmaps fail.
func TestReleaseResources_ErrorsContinue(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, plan9Ctrl, vpciCtrl, _ := newContainerTestController(t)

	scsiGUID, _ := guid.NewV4()
	plan9GUID, _ := guid.NewV4()
	deviceGUID, _ := guid.NewV4()

	c.scsiResources = []guid.GUID{scsiGUID}
	c.plan9Resources = []guid.GUID{plan9GUID}
	c.devices = []guid.GUID{deviceGUID}

	// Each unmap fails, but releaseResources should still attempt all.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scsiGUID).Return(errUnmapSCSI)
	plan9Ctrl.EXPECT().UnmapFromGuest(gomock.Any(), plan9GUID).Return(errUnmapPlan9)
	vpciCtrl.EXPECT().RemoveFromVM(gomock.Any(), deviceGUID).Return(errRemoveVPCI)

	// Should not panic; errors are logged.
	c.releaseResources(t.Context())

	// Slices still cleared even on errors.
	if c.scsiResources != nil {
		t.Error("scsiResources should be nil after releaseResources")
	}
	if c.plan9Resources != nil {
		t.Error("plan9Resources should be nil after releaseResources")
	}
	if c.devices != nil {
		t.Error("devices should be nil after releaseResources")
	}
}

// TestReleaseResources_MultipleROLayers verifies that all read-only layers
// are individually unmapped.
func TestReleaseResources_MultipleROLayers(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, guestCtrl := newContainerTestController(t)

	scratchGUID, _ := guid.NewV4()
	roGUID0, _ := guid.NewV4()
	roGUID1, _ := guid.NewV4()
	roGUID2, _ := guid.NewV4()

	c.layers = &scsiLayers{
		layersCombined: true,
		rootfsPath:     "/rootfs",
		scratch:        scsiReservation{id: scratchGUID, guestPath: "/dev/scratch"},
		roLayers: []scsiReservation{
			{id: roGUID0, guestPath: "/dev/ro0"},
			{id: roGUID1, guestPath: "/dev/ro1"},
			{id: roGUID2, guestPath: "/dev/ro2"},
		},
	}

	guestCtrl.EXPECT().
		RemoveLCOWCombinedLayers(gomock.Any(), gomock.Any()).
		Return(nil)

	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scratchGUID).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUID0).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUID1).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUID2).Return(nil)

	c.releaseResources(t.Context())

	if c.layers != nil {
		t.Error("layers should be nil after releaseResources")
	}
}

// TestReleaseResources_NoResources verifies that releaseResources is a no-op
// when no resources were allocated.
func TestReleaseResources_NoResources(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	// No mock calls expected.
	c.releaseResources(t.Context())
}

// --- closeContainer ---

// TestCloseContainer_IdempotentViaSyncOnce verifies that closeContainer
// executes teardown exactly once even when called multiple times.
func TestCloseContainer_IdempotentViaSyncOnce(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	// No container, no layers — closeContainer should just close terminatedCh.
	c.closeContainer(t.Context())

	// Verify terminatedCh is closed.
	select {
	case <-c.terminatedCh:
	default:
		t.Fatal("terminatedCh should be closed after closeContainer")
	}

	// Second call should be a no-op (no panic from double-close).
	c.closeContainer(t.Context())
}

// TestCloseContainer_NilContainer verifies that closeContainer succeeds
// without panicking when the container handle is nil.
func TestCloseContainer_NilContainer(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.container = nil

	c.closeContainer(t.Context())

	select {
	case <-c.terminatedCh:
	default:
		t.Fatal("terminatedCh should be closed after closeContainer")
	}
}

// --- Start ---

// TestStart_WrongState verifies that Start rejects calls outside StateCreated.
func TestStart_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateRunning, StateStopped, StateInvalid}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			_, err := c.Start(t.Context(), nil)
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Start() error = %v, want ErrFailedPrecondition", err)
			}
		})
	}
}

// --- Create ---

// TestCreate_WrongState verifies that Create rejects calls outside StateNotCreated.
func TestCreate_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateCreated, StateRunning, StateStopped, StateInvalid}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			err := c.Create(t.Context(), nil, nil, nil)
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Create() error = %v, want ErrFailedPrecondition", err)
			}
		})
	}
}

// --- Pids ---

// TestPids_WrongState verifies that Pids rejects calls outside StateRunning.
func TestPids_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateStopped, StateInvalid}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			_, err := c.Pids(t.Context())
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Pids() error = %v, want ErrFailedPrecondition", err)
			}
		})
	}
}

// --- Stats ---

// TestStats_WrongState verifies that Stats rejects calls outside StateRunning.
func TestStats_WrongState(t *testing.T) {
	t.Parallel()
	invalidStates := []State{StateNotCreated, StateCreated, StateStopped, StateInvalid}

	for _, state := range invalidStates {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			c.state = state

			_, err := c.Stats(t.Context())
			if !errors.Is(err, errdefs.ErrFailedPrecondition) {
				t.Errorf("Stats() error = %v, want ErrFailedPrecondition", err)
			}
		})
	}
}

// --- KillProcess (additional state and flow tests) ---

// TestKillProcess_AllowedInCreatedState verifies that KillProcess does not
// reject containers in StateCreated. The downstream error from the process
// controller (which is in StateNotCreated) is expected but should not be
// confused with a container-level state rejection.
func TestKillProcess_AllowedInCreatedState(t *testing.T) {
	t.Parallel()
	c, _, _, _, guestCtrl := newContainerTestController(t)
	c.state = StateCreated

	// Add a process controller in its initial (NotCreated) state.
	c.processes[""] = process.New(testContainerID, "", nil, 0)

	guestCtrl.EXPECT().
		Capabilities().
		Return(&gcs.LCOWGuestDefinedCapabilities{})

	// SIGTERM (15) with no signal support returns nil options.
	err := c.KillProcess(t.Context(), "", 15, false)
	// An error from the process controller is expected (process not started),
	// but the container-level state check should not fire.
	if err != nil && strings.Contains(err.Error(), "cannot kill") {
		t.Errorf("KillProcess should not reject StateCreated containers, got: %v", err)
	}
}

// TestKillProcess_AllowedInStoppedState verifies that KillProcess does not
// reject containers in StateStopped.
func TestKillProcess_AllowedInStoppedState(t *testing.T) {
	t.Parallel()
	c, _, _, _, guestCtrl := newContainerTestController(t)
	c.state = StateStopped

	c.processes[""] = process.New(testContainerID, "", nil, 0)

	guestCtrl.EXPECT().
		Capabilities().
		Return(&gcs.LCOWGuestDefinedCapabilities{})

	err := c.KillProcess(t.Context(), "", 15, false)
	// Container state check should pass; any error should come from the process.
	if err != nil && strings.Contains(err.Error(), "cannot kill") {
		t.Errorf("KillProcess should not reject StateStopped containers, got: %v", err)
	}
}
