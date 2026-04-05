//go:build windows && lcow

package plan9

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount"
	mountmocks "github.com/Microsoft/hcsshim/internal/controller/device/plan9/mount/mocks"
	"github.com/Microsoft/hcsshim/internal/controller/device/plan9/share"
	sharemocks "github.com/Microsoft/hcsshim/internal/controller/device/plan9/share/mocks"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

var (
	errVMAdd    = errors.New("VM add failed")
	errVMRemove = errors.New("VM remove failed")
	errMount    = errors.New("guest mount failed")
	errUnmount  = errors.New("guest unmount failed")
)

// ─────────────────────────────────────────────────────────────────────────────
// Helper interfaces and test controller setup
// ─────────────────────────────────────────────────────────────────────────────

// combinedVM satisfies vmPlan9 (share.VMPlan9Adder + share.VMPlan9Remover)
// by delegating to the individual sharemocks so each can carry independent expectations.
type combinedVM struct {
	add    *sharemocks.MockVMPlan9Adder
	remove *sharemocks.MockVMPlan9Remover
}

func (v *combinedVM) AddPlan9(ctx context.Context, settings hcsschema.Plan9Share) error {
	return v.add.AddPlan9(ctx, settings)
}

func (v *combinedVM) RemovePlan9(ctx context.Context, settings hcsschema.Plan9Share) error {
	return v.remove.RemovePlan9(ctx, settings)
}

// combinedGuest satisfies guestPlan9 (LinuxGuestPlan9Mounter + LinuxGuestPlan9Unmounter).
type combinedGuest struct {
	mounter   *mountmocks.MockLinuxGuestPlan9Mounter
	unmounter *mountmocks.MockLinuxGuestPlan9Unmounter
}

func (g *combinedGuest) AddLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error {
	return g.mounter.AddLCOWMappedDirectory(ctx, settings)
}

func (g *combinedGuest) RemoveLCOWMappedDirectory(ctx context.Context, settings guestresource.LCOWMappedDirectory) error {
	return g.unmounter.RemoveLCOWMappedDirectory(ctx, settings)
}

type testController struct {
	ctx          context.Context
	c            *Controller
	vmAdd        *sharemocks.MockVMPlan9Adder
	vmRemove     *sharemocks.MockVMPlan9Remover
	guestMount   *mountmocks.MockLinuxGuestPlan9Mounter
	guestUnmount *mountmocks.MockLinuxGuestPlan9Unmounter
}

func newTestController(t *testing.T, noWritableFileShares bool) *testController {
	t.Helper()

	ctrl := gomock.NewController(t)
	vmAdd := sharemocks.NewMockVMPlan9Adder(ctrl)
	vmRemove := sharemocks.NewMockVMPlan9Remover(ctrl)
	guestMount := mountmocks.NewMockLinuxGuestPlan9Mounter(ctrl)
	guestUnmount := mountmocks.NewMockLinuxGuestPlan9Unmounter(ctrl)

	vm := &combinedVM{add: vmAdd, remove: vmRemove}
	guest := &combinedGuest{mounter: guestMount, unmounter: guestUnmount}

	return &testController{
		ctx:          context.Background(),
		c:            New(vm, guest, noWritableFileShares),
		vmAdd:        vmAdd,
		vmRemove:     vmRemove,
		guestMount:   guestMount,
		guestUnmount: guestUnmount,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Reserve tests
// ─────────────────────────────────────────────────────────────────────────────

// TestReserve_NewShare verifies that reserving a new host path creates a share
// entry, returns a non-empty guest path, and stores a unique reservation ID.
func TestReserve_NewShare(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, guestPath, err := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guest path")
	}
	if len(tc.c.reservations) != 1 {
		t.Errorf("expected 1 reservation, got %d", len(tc.c.reservations))
	}
	if len(tc.c.sharesByHostPath) != 1 {
		t.Errorf("expected 1 share, got %d", len(tc.c.sharesByHostPath))
	}
	if _, ok := tc.c.reservations[id]; !ok {
		t.Error("returned ID not present in reservations map")
	}
}

// TestReserve_SameHostPath_RefsExistingShare verifies that two reservations for
// the same host path reuse the existing share (only one share entry) and return
// distinct reservation IDs but the same guest path.
func TestReserve_SameHostPath_RefsExistingShare(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id1, gp1, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})
	id2, gp2, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	if id1 == id2 {
		t.Error("expected different reservation IDs for two callers on the same path")
	}
	if gp1 != gp2 {
		t.Errorf("expected same guest path for same host path, got %q vs %q", gp1, gp2)
	}
	if len(tc.c.sharesByHostPath) != 1 {
		t.Errorf("expected 1 share for same host path, got %d", len(tc.c.sharesByHostPath))
	}
	if len(tc.c.reservations) != 2 {
		t.Errorf("expected 2 reservations, got %d", len(tc.c.reservations))
	}
}

// TestReserve_DifferentHostPaths_CreatesSeparateShares verifies that reserving
// two distinct host paths creates two independent share entries.
func TestReserve_DifferentHostPaths_CreatesSeparateShares(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	_, _, _ = tc.c.Reserve(tc.ctx, share.Config{HostPath: "/path/a"}, mount.Config{})
	_, _, _ = tc.c.Reserve(tc.ctx, share.Config{HostPath: "/path/b"}, mount.Config{})

	if len(tc.c.sharesByHostPath) != 2 {
		t.Errorf("expected 2 shares, got %d", len(tc.c.sharesByHostPath))
	}
}

// TestReserve_DifferentConfig_SameHostPath_Errors verifies that attempting to
// reserve a host path with a different config (e.g., ReadOnly differs) when a
// share already exists for that path returns an error.
func TestReserve_DifferentConfig_SameHostPath_Errors(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	_, _, _ = tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	// Same host path but read-only flag differs.
	_, _, err := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path", ReadOnly: true}, mount.Config{})
	if err == nil {
		t.Fatal("expected error when re-reserving same host path with different config")
	}
}

// TestReserve_WritableDenied verifies that when the controller is constructed
// with noWritableFileShares=true, reserving a writable share is rejected before
// any share or mount state is created.
func TestReserve_WritableDenied(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, true /* noWritableFileShares */)

	_, _, err := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path" /* writable */}, mount.Config{})
	if err == nil {
		t.Fatal("expected error when adding writable share with noWritableFileShares=true")
	}
}

// TestReserve_ReadOnlyAllowedWhenWritableDenied verifies that read-only shares
// are still permitted when noWritableFileShares=true.
func TestReserve_ReadOnlyAllowedWhenWritableDenied(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, true /* noWritableFileShares */)

	_, _, err := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path", ReadOnly: true /* readOnly */}, mount.Config{ReadOnly: true})
	if err != nil {
		t.Fatalf("unexpected error for read-only share: %v", err)
	}
}

// TestReserve_NameCounterIncrements verifies that each new share created for a
// distinct host path increments the controller's nameCounter.
func TestReserve_NameCounterIncrements(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	_, _, _ = tc.c.Reserve(tc.ctx, share.Config{HostPath: "/path/a"}, mount.Config{})
	_, _, _ = tc.c.Reserve(tc.ctx, share.Config{HostPath: "/path/b"}, mount.Config{})

	if tc.c.nameCounter != 2 {
		t.Errorf("expected nameCounter=2, got %d", tc.c.nameCounter)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// MapToGuest tests
// ─────────────────────────────────────────────────────────────────────────────

// TestMapToGuest_HappyPath verifies a normal Reserve → MapToGuest flow: the
// share is added to the VM and mounted in the guest, returning a non-empty
// guest path that matches the one returned during Reserve.
func TestMapToGuest_HappyPath(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, gp, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)

	guestPath, err := tc.c.MapToGuest(tc.ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guestPath == "" {
		t.Error("expected non-empty guest path")
	}
	if guestPath != gp {
		t.Errorf("expected guest path %q from MapToGuest to match reservation guest path %q", guestPath, gp)
	}
}

// TestMapToGuest_VMAddFails_Errors verifies that when the host-side AddPlan9
// fails, the error propagates and no guest mount is attempted.
func TestMapToGuest_VMAddFails_Errors(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(errVMAdd)

	_, err := tc.c.MapToGuest(tc.ctx, id)
	if err == nil {
		t.Fatal("expected error when VM add fails")
	}
}

// TestMapToGuest_GuestMountFails_RetryMapToGuest_Fails verifies the "forward"
// recovery path after a guest mount failure. Because the mount is now in the
// terminal unmounted state, a subsequent MapToGuest with the same reservation
// also fails. The caller must use UnmapFromGuest to clean up.
func TestMapToGuest_GuestMountFails_RetryMapToGuest_Fails(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	// First MapToGuest: VM add succeeds, guest mount fails.
	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errMount)

	_, err := tc.c.MapToGuest(tc.ctx, id)
	if err == nil {
		t.Fatal("expected error when guest mount fails")
	}

	// Forward retry: MapToGuest again with the same reservation.
	// AddToVM is idempotent (share already in StateAdded), but MountToGuest
	// fails because the mount is in the terminal StateUnmounted.
	_, err = tc.c.MapToGuest(tc.ctx, id)
	if err == nil {
		t.Fatal("expected error on retry MapToGuest after terminal mount failure")
	}

	// Reservation should still exist — caller must call UnmapFromGuest.
	if _, ok := tc.c.reservations[id]; !ok {
		t.Error("reservation should still exist after failed retry")
	}
}

// TestMapToGuest_GuestMountFails_UnmapFromGuest_CleansUp verifies the "backward"
// recovery path after a guest mount failure. The caller invokes UnmapFromGuest
// which skips the guest unmount (mount was never established), removes the share
// from the VM, and cleans up both the reservation and share entries.
func TestMapToGuest_GuestMountFails_UnmapFromGuest_CleansUp(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	// MapToGuest: VM add succeeds, guest mount fails.
	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errMount)

	_, err := tc.c.MapToGuest(tc.ctx, id)
	if err == nil {
		t.Fatal("expected error when guest mount fails")
	}

	// Backward path: UnmapFromGuest cleans up the share from the VM.
	// No guest unmount is expected (mount was never established).
	// VM remove IS expected (share was added to VM successfully).
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)

	if err := tc.c.UnmapFromGuest(tc.ctx, id); err != nil {
		t.Fatalf("UnmapFromGuest after failed mount: %v", err)
	}

	// Everything should be cleaned up.
	if len(tc.c.reservations) != 0 {
		t.Errorf("expected 0 reservations after cleanup, got %d", len(tc.c.reservations))
	}
	if len(tc.c.sharesByHostPath) != 0 {
		t.Errorf("expected 0 shares after cleanup, got %d", len(tc.c.sharesByHostPath))
	}
}

// TestMapToGuest_SharedPath_VMAddCalledOnce verifies that when two reservations
// share the same host path, AddPlan9 and AddLCOWMappedDirectory are each called
// exactly once — the second MapToGuest is a no-op that returns the existing
// guest path.
func TestMapToGuest_SharedPath_VMAddCalledOnce(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id1, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})
	id2, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	// AddPlan9 and AddLCOWMappedDirectory each called exactly once.
	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	gp1, err := tc.c.MapToGuest(tc.ctx, id1)
	if err != nil {
		t.Fatalf("MapToGuest id1: %v", err)
	}
	gp2, err := tc.c.MapToGuest(tc.ctx, id2)
	if err != nil {
		t.Fatalf("MapToGuest id2: %v", err)
	}
	if gp1 != gp2 {
		t.Errorf("expected same guest path for shared mount, got %q vs %q", gp1, gp2)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UnmapFromGuest tests
// ─────────────────────────────────────────────────────────────────────────────

// TestUnmapFromGuest_HappyPath verifies the successful teardown of a fully
// mapped share: the guest unmount and VM removal are issued, and both the
// reservation and share entries are cleaned up.
func TestUnmapFromGuest_HappyPath(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = tc.c.MapToGuest(tc.ctx, id)

	tc.guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)

	if err := tc.c.UnmapFromGuest(tc.ctx, id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tc.c.reservations) != 0 {
		t.Errorf("expected 0 reservations after unmap, got %d", len(tc.c.reservations))
	}
	if len(tc.c.sharesByHostPath) != 0 {
		t.Errorf("expected 0 shares after unmap, got %d", len(tc.c.sharesByHostPath))
	}
}

// TestUnmapFromGuest_GuestUnmountFails_Retryable verifies that a failed guest
// unmount leaves the reservation intact so the caller can retry. On a
// successful retry the mount is unmounted and the share is removed from the VM.
func TestUnmapFromGuest_GuestUnmountFails_Retryable(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = tc.c.MapToGuest(tc.ctx, id)

	// First unmap: guest unmount fails.
	tc.guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(errUnmount)
	if err := tc.c.UnmapFromGuest(tc.ctx, id); err == nil {
		t.Fatal("expected error on failed guest unmount")
	}
	if _, ok := tc.c.reservations[id]; !ok {
		t.Error("reservation should remain for retry after failed unmount")
	}

	// Retry succeeds.
	tc.guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)
	if err := tc.c.UnmapFromGuest(tc.ctx, id); err != nil {
		t.Fatalf("retry UnmapFromGuest failed: %v", err)
	}
}

// TestUnmapFromGuest_VMRemoveFails_Retryable verifies that when the guest
// unmount succeeds but VM removal fails, the reservation is preserved for
// retry. On retry only VM removal is re-attempted — the guest unmount is not
// re-issued.
func TestUnmapFromGuest_VMRemoveFails_Retryable(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = tc.c.MapToGuest(tc.ctx, id)

	// First unmap: guest unmount succeeds, VM remove fails.
	tc.guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(errVMRemove)
	if err := tc.c.UnmapFromGuest(tc.ctx, id); err == nil {
		t.Fatal("expected error on failed VM remove")
	}
	if _, ok := tc.c.reservations[id]; !ok {
		t.Error("reservation should remain for retry after failed VM remove")
	}

	// Retry succeeds.
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)
	if err := tc.c.UnmapFromGuest(tc.ctx, id); err != nil {
		t.Fatalf("retry UnmapFromGuest failed: %v", err)
	}
}

// TestUnmapFromGuest_RefCounting_VMRemoveOnLastRef verifies that with two
// reservations on the same path, the first UnmapFromGuest only decrements the
// ref count (no guest/VM calls), and the second issues the physical guest
// unmount and VM removal.
func TestUnmapFromGuest_RefCounting_VMRemoveOnLastRef(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	id1, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})
	id2, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	_, _ = tc.c.MapToGuest(tc.ctx, id1)
	_, _ = tc.c.MapToGuest(tc.ctx, id2)

	// First unmap: ref drops to 1 — no VM or guest calls.
	if err := tc.c.UnmapFromGuest(tc.ctx, id1); err != nil {
		t.Fatalf("first UnmapFromGuest: %v", err)
	}
	if len(tc.c.sharesByHostPath) != 1 {
		t.Errorf("share should still exist after first unmap, got %d shares", len(tc.c.sharesByHostPath))
	}

	// Second unmap: last ref — guest unmount and VM remove issued.
	tc.guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	if err := tc.c.UnmapFromGuest(tc.ctx, id2); err != nil {
		t.Fatalf("second UnmapFromGuest: %v", err)
	}
	if len(tc.c.sharesByHostPath) != 0 {
		t.Errorf("expected 0 shares after last unmap, got %d", len(tc.c.sharesByHostPath))
	}
}

// TestUnmapFromGuest_WithoutMapToGuest_CleansUp verifies that calling
// UnmapFromGuest on a reservation that was never passed to MapToGuest still
// cleans up correctly without issuing any VM or guest calls.
func TestUnmapFromGuest_WithoutMapToGuest_CleansUp(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	// Reserve but never MapToGuest — no VM or guest calls expected.
	id, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})

	if err := tc.c.UnmapFromGuest(tc.ctx, id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tc.c.reservations) != 0 {
		t.Errorf("expected 0 reservations, got %d", len(tc.c.reservations))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Full lifecycle tests
// ─────────────────────────────────────────────────────────────────────────────

// TestFullLifecycle_ReuseAfterRelease verifies that after a full Reserve →
// MapToGuest → UnmapFromGuest cycle, the same host path can be reserved again
// as a fresh share with a new reservation ID.
func TestFullLifecycle_ReuseAfterRelease(t *testing.T) {
	t.Parallel()
	tc := newTestController(t, false)

	// First full cycle.
	id1, _, _ := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})
	tc.vmAdd.EXPECT().AddPlan9(gomock.Any(), gomock.Any()).Return(nil)
	tc.guestMount.EXPECT().AddLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	_, _ = tc.c.MapToGuest(tc.ctx, id1)
	tc.guestUnmount.EXPECT().RemoveLCOWMappedDirectory(gomock.Any(), gomock.Any()).Return(nil)
	tc.vmRemove.EXPECT().RemovePlan9(gomock.Any(), gomock.Any()).Return(nil)
	_ = tc.c.UnmapFromGuest(tc.ctx, id1)

	// Reserve the same path again — should start a fresh share.
	id2, _, err := tc.c.Reserve(tc.ctx, share.Config{HostPath: "/host/path"}, mount.Config{})
	if err != nil {
		t.Fatalf("re-reserve after release: %v", err)
	}
	if id1 == id2 {
		t.Error("expected a new reservation ID after re-reserving a released path")
	}
}
