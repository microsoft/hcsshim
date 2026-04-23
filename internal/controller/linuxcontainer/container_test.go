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
	errUnmapSCSI = errors.New("unmap scsi failed")
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

// TestListProcesses_EmptyAndInitOnly verifies that ListProcesses returns an
// empty map both when the processes map is empty and when only the init
// process is registered.
func TestListProcesses_EmptyAndInitOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		seedMap func(c *Controller)
	}{
		{name: "no processes", seedMap: func(*Controller) {}},
		{
			name: "init only",
			seedMap: func(c *Controller) {
				c.processes[""] = process.New(testContainerID, "", nil, 0)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _, _, _, _ := newContainerTestController(t)
			tc.seedMap(c)

			result, err := c.ListProcesses()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result) != 0 {
				t.Errorf("expected 0 exec processes, got %d", len(result))
			}
		})
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

// TestDeleteProcess_InitProcessInNotCreatedStateRejected verifies that
// deleting the init process while the underlying process controller is in
// its initial (NotCreated) state returns an error from the process layer.
func TestDeleteProcess_InitProcessInNotCreatedStateRejected(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)
	c.state = StateCreated

	// process.New starts in StateNotCreated; Delete from NotCreated returns
	// ErrFailedPrecondition from the process package.
	c.processes[""] = process.New(testContainerID, "", nil, 0)

	_, err := c.DeleteProcess(t.Context(), "")
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Errorf("DeleteProcess() error = %v, want ErrFailedPrecondition", err)
	}

	// The process entry must be retained so a retry can locate it.
	if _, ok := c.processes[""]; !ok {
		t.Error("init process entry should be retained after failed delete")
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
		RemoveCombinedLayers(gomock.Any(), guestresource.LCOWCombinedLayers{
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

	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("releaseResources returned error: %v", err)
	}

	// After successful release, layersCombined is reset and scratch zeroed.
	if c.layers == nil {
		t.Fatal("layers struct should still be present after releaseResources")
	}
	if c.layers.layersCombined {
		t.Error("layersCombined should be false after releaseResources")
	}
	var zeroGUID guid.GUID
	if c.layers.scratch.id != zeroGUID {
		t.Error("scratch reservation should be zeroed after releaseResources")
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

	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("releaseResources returned error: %v", err)
	}
}

// TestReleaseResources_LayersNotCombined verifies that when layers exist but
// were not combined, RemoveCombinedLayers is not called.
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

	// Only unmaps expected; no RemoveCombinedLayers call.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scratchGUID).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUID).Return(nil)

	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("releaseResources returned error: %v", err)
	}
}

// TestReleaseResources_Idempotent verifies that releaseResources can be safely
// invoked multiple times without panicking.
func TestReleaseResources_Idempotent(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	scsiGUID, _ := guid.NewV4()
	c.scsiResources = []guid.GUID{scsiGUID}

	// The implementation does not clear the slice on success, so a second
	// call will retry the unmap. Both calls succeed.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scsiGUID).Return(nil).Times(2)

	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("first releaseResources returned error: %v", err)
	}
	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("second releaseResources returned error: %v", err)
	}
}

// TestReleaseResources_StopsOnFirstError verifies that releaseResources
// returns the first error encountered and does not proceed to subsequent
// resource categories.
func TestReleaseResources_StopsOnFirstError(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	scsiGUID, _ := guid.NewV4()
	plan9GUID, _ := guid.NewV4()
	deviceGUID, _ := guid.NewV4()

	c.scsiResources = []guid.GUID{scsiGUID}
	c.plan9Resources = []guid.GUID{plan9GUID}
	c.devices = []guid.GUID{deviceGUID}

	// SCSI unmap fails; plan9/vpci should not be invoked.
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scsiGUID).Return(errUnmapSCSI)

	err := c.releaseResources(t.Context())
	if !errors.Is(err, errUnmapSCSI) {
		t.Fatalf("releaseResources error = %v, want %v", err, errUnmapSCSI)
	}

	// Failed scsi entry is retained for retry; plan9 and devices unchanged.
	if len(c.scsiResources) != 1 {
		t.Errorf("scsiResources len = %d, want 1 (retained for retry)", len(c.scsiResources))
	}
	if len(c.plan9Resources) != 1 {
		t.Errorf("plan9Resources len = %d, want 1 (untouched)", len(c.plan9Resources))
	}
	if len(c.devices) != 1 {
		t.Errorf("devices len = %d, want 1 (untouched)", len(c.devices))
	}
}

// TestReleaseResources_MultipleROLayers verifies that all read-only layers
// are individually unmapped.
func TestReleaseResources_MultipleROLayers(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, guestCtrl := newContainerTestController(t)

	scratchGUID, _ := guid.NewV4()
	roGUIDs := [3]guid.GUID{}
	for i := range roGUIDs {
		roGUIDs[i], _ = guid.NewV4()
	}

	c.layers = &scsiLayers{
		layersCombined: true,
		rootfsPath:     "/rootfs",
		scratch:        scsiReservation{id: scratchGUID, guestPath: "/dev/scratch"},
		roLayers: []scsiReservation{
			{id: roGUIDs[0], guestPath: "/dev/ro0"},
			{id: roGUIDs[1], guestPath: "/dev/ro1"},
			{id: roGUIDs[2], guestPath: "/dev/ro2"},
		},
	}

	guestCtrl.EXPECT().RemoveCombinedLayers(gomock.Any(), gomock.Any()).Return(nil)
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scratchGUID).Return(nil)
	for _, g := range roGUIDs {
		scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), g).Return(nil)
	}

	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("releaseResources returned error: %v", err)
	}
}

// TestReleaseResources_RemoveCombinedLayersFails verifies that a failure to
// remove combined layers aborts release and leaves layersCombined=true so a
// retry can resume from the same step.
func TestReleaseResources_RemoveCombinedLayersFails(t *testing.T) {
	t.Parallel()
	c, _, _, _, guestCtrl := newContainerTestController(t)

	scratchGUID, _ := guid.NewV4()
	c.layers = &scsiLayers{
		layersCombined: true,
		rootfsPath:     "/rootfs",
		scratch:        scsiReservation{id: scratchGUID, guestPath: "/dev/scratch"},
	}

	wantErr := errors.New("remove combined layers failed")
	guestCtrl.EXPECT().RemoveCombinedLayers(gomock.Any(), gomock.Any()).Return(wantErr)

	err := c.releaseResources(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("releaseResources error = %v, want %v", err, wantErr)
	}

	// State must be preserved so the next call retries the same step.
	if !c.layers.layersCombined {
		t.Error("layersCombined should remain true after failed removal")
	}
	if c.layers.scratch.id != scratchGUID {
		t.Error("scratch reservation should be untouched after failed removal")
	}
}

// TestReleaseResources_ScratchUnmapFails verifies that a scratch unmap
// failure leaves the scratch reservation intact for retry.
func TestReleaseResources_ScratchUnmapFails(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	scratchGUID, _ := guid.NewV4()
	c.layers = &scsiLayers{
		scratch: scsiReservation{id: scratchGUID, guestPath: "/dev/scratch"},
	}

	wantErr := errors.New("scratch unmap failed")
	scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), scratchGUID).Return(wantErr)

	err := c.releaseResources(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("releaseResources error = %v, want %v", err, wantErr)
	}
	if c.layers.scratch.id != scratchGUID {
		t.Error("scratch reservation should be retained for retry after failure")
	}
}

// TestReleaseResources_ROLayerUnmapMidwayFails verifies that when an RO layer
// unmap fails mid-iteration, the failed entry and subsequent entries are
// retained while already-unmapped entries are dropped.
func TestReleaseResources_ROLayerUnmapMidwayFails(t *testing.T) {
	t.Parallel()
	c, scsiCtrl, _, _, _ := newContainerTestController(t)

	roGUIDs := [3]guid.GUID{}
	for i := range roGUIDs {
		roGUIDs[i], _ = guid.NewV4()
	}

	c.layers = &scsiLayers{
		roLayers: []scsiReservation{
			{id: roGUIDs[0]},
			{id: roGUIDs[1]},
			{id: roGUIDs[2]},
		},
	}

	wantErr := errors.New("ro layer unmap failed")
	gomock.InOrder(
		scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUIDs[0]).Return(nil),
		scsiCtrl.EXPECT().UnmapFromGuest(gomock.Any(), roGUIDs[1]).Return(wantErr),
	)

	err := c.releaseResources(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("releaseResources error = %v, want %v", err, wantErr)
	}

	// Tail beginning at the failed index must be retained.
	if len(c.layers.roLayers) != 2 {
		t.Fatalf("roLayers len = %d, want 2 (failed + tail retained)", len(c.layers.roLayers))
	}
	if c.layers.roLayers[0].id != roGUIDs[1] || c.layers.roLayers[1].id != roGUIDs[2] {
		t.Errorf("roLayers content unexpected: %+v", c.layers.roLayers)
	}
}

// TestReleaseResources_NoResources verifies that releaseResources is a no-op
// when no resources were allocated.
func TestReleaseResources_NoResources(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	// No mock calls expected.
	if err := c.releaseResources(t.Context()); err != nil {
		t.Fatalf("releaseResources returned error: %v", err)
	}
}

// --- closeContainer ---

// TestCloseContainer_IdempotentViaFlags verifies that closeContainer is
// safe to call multiple times: terminatedCh is closed exactly once and
// repeated calls are no-ops when the container handle is nil.
func TestCloseContainer_IdempotentViaFlags(t *testing.T) {
	t.Parallel()
	c, _, _, _, _ := newContainerTestController(t)

	// No container, no layers — closeContainer should just close terminatedCh.
	if err := c.closeContainer(t.Context()); err != nil {
		t.Fatalf("closeContainer returned error: %v", err)
	}

	// Verify terminatedCh is closed.
	select {
	case <-c.terminatedCh:
	default:
		t.Fatal("terminatedCh should be closed after closeContainer")
	}

	// Second call must be a no-op (no panic from double-close on terminatedCh).
	if err := c.closeContainer(t.Context()); err != nil {
		t.Fatalf("second closeContainer returned error: %v", err)
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

// TestKillProcess_AllowedInPostCreatedStates verifies that KillProcess does
// not reject containers in StateCreated or StateStopped on container-level
// state grounds. Errors that surface from the underlying process controller
// (which here is in StateNotCreated) are tolerated; the test only asserts
// that the container's own "cannot kill" precondition does not fire.
func TestKillProcess_AllowedInPostCreatedStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state State
	}{
		{name: "created", state: StateCreated},
		{name: "stopped", state: StateStopped},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _, _, _, guestCtrl := newContainerTestController(t)
			c.state = tc.state
			c.processes[""] = process.New(testContainerID, "", nil, 0)

			guestCtrl.EXPECT().
				Capabilities().
				Return(&gcs.LCOWGuestDefinedCapabilities{})

			// SIGTERM (15) with no signal support returns nil options.
			err := c.KillProcess(t.Context(), "", 15, false)
			if err != nil && strings.Contains(err.Error(), "cannot kill") {
				t.Errorf("KillProcess should not reject %s containers, got: %v", tc.state, err)
			}
		})
	}
}
