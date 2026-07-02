//go:build windows && lcow

package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/controller/migration/mocks"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/pkg/migration"
)

// waitForState polls until the controller reaches want or the deadline elapses,
// so background transfer goroutines can be observed deterministically.
func waitForState(t *testing.T, c *Controller, want State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.State() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state = %s; want %s", c.State(), want)
}

// ─────────────────────────────────────────────────────────────────────────────
// New / State
// ─────────────────────────────────────────────────────────────────────────────

// TestNew verifies a fresh controller starts idle and ready to host a session.
func TestNew(t *testing.T) {
	c := New()
	if c.State() != StateIdle {
		t.Errorf("expected state Idle, got %s", c.State())
	}
	if c.socketReady == nil {
		t.Error("expected socketReady channel to be initialized")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// sessionIDToUint32
// ─────────────────────────────────────────────────────────────────────────────

// TestSessionIDToUint32 verifies the mapping is deterministic and that a GUID
// maps to the same value regardless of letter case.
func TestSessionIDToUint32(t *testing.T) {
	const lower = "5f0b1190-63be-4e0c-b974-bd0f55675a42"
	const upper = "5F0B1190-63BE-4E0C-B974-BD0F55675A42"

	if got, want := sessionIDToUint32(lower), sessionIDToUint32(lower); got != want {
		t.Errorf("not deterministic: %d != %d", got, want)
	}
	if got, want := sessionIDToUint32(upper), sessionIDToUint32(lower); got != want {
		t.Errorf("GUID case affected result: %d != %d", got, want)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Transfer
// ─────────────────────────────────────────────────────────────────────────────

// TestTransfer_RejectsSessionMismatch verifies a call for a different session is
// rejected.
func TestTransfer_RejectsSessionMismatch(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"

	if err := c.Transfer(t.Context(), "other", time.Minute); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

// TestTransfer_NoopWhenInProgress verifies a transfer that is already waiting,
// running, or done is a no-op that leaves the state unchanged.
func TestTransfer_NoopWhenInProgress(t *testing.T) {
	for _, st := range []State{StateSocketWaiting, StateTransferring, StateTransferCompleted} {
		t.Run(st.String(), func(t *testing.T) {
			c := New()
			c.sessionID = "sess-1"
			c.state = st

			if err := c.Transfer(t.Context(), "sess-1", time.Minute); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.State() != st {
				t.Errorf("state = %s; want unchanged %s", c.State(), st)
			}
		})
	}
}

// TestTransfer_RejectsInvalidState verifies a transfer cannot start from a state
// that has no path to it.
func TestTransfer_RejectsInvalidState(t *testing.T) {
	c := New() // StateIdle

	if err := c.Transfer(t.Context(), "", time.Minute); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestTransfer_ClaimsSocketWaiting verifies that starting a transfer before the
// socket arrives claims the wait so later calls no-op.
func TestTransfer_ClaimsSocketWaiting(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"
	c.state = StateSourceExported

	// Long timeout so the background goroutine simply parks on the socket.
	if err := c.Transfer(t.Context(), "sess-1", time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State() != StateSocketWaiting {
		t.Errorf("state = %s; want SocketWaiting", c.State())
	}

	// Unpark the goroutine so it exits without driving a transfer.
	close(c.socketReady)
}

// TestTransfer_Success verifies that with the socket ready the background
// transfer runs to completion.
func TestTransfer_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().StartLiveMigrationOnSource(gomock.Any(), gomock.Any()).Return(nil)
	vm.EXPECT().StartLiveMigrationTransfer(gomock.Any(), gomock.Any()).Return(nil)

	c := New()
	c.sessionID = "sess-1"
	c.origin = hcsschema.MigrationOriginSource
	c.vmController = vm
	c.state = StateSocketReady
	close(c.socketReady) // socket already arrived

	if err := c.Transfer(t.Context(), "sess-1", time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForState(t, c, StateTransferCompleted)
}

// TestTransfer_SocketTimeout verifies the session fails if the socket never
// arrives within the timeout.
func TestTransfer_SocketTimeout(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"
	c.state = StateSocketReady // socketReady stays open so the wait times out

	if err := c.Transfer(t.Context(), "sess-1", time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForState(t, c, StateFailed)
}

// ─────────────────────────────────────────────────────────────────────────────
// Finalize
// ─────────────────────────────────────────────────────────────────────────────

// TestFinalize_RejectsUnspecifiedAction verifies an unspecified action is refused.
func TestFinalize_RejectsUnspecifiedAction(t *testing.T) {
	c := New()

	err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_UNSPECIFIED, nil)
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

// TestFinalize_IdempotentFinalized verifies finalizing an already-finalized
// session is a no-op.
func TestFinalize_IdempotentFinalized(t *testing.T) {
	c := New()
	c.state = StateFinalized

	if err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_RESUME, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFinalize_RejectsWrongState verifies finalize is rejected before a transfer
// has completed or been cancelled.
func TestFinalize_RejectsWrongState(t *testing.T) {
	c := New() // StateIdle

	err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_RESUME, nil)
	if !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestFinalize_RejectsUnsupportedAction verifies an unknown action is rejected.
func TestFinalize_RejectsUnsupportedAction(t *testing.T) {
	c := New()
	c.state = StateTransferCompleted

	err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction(99), nil)
	if !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

// TestFinalize_FinalizeError verifies a failure finalizing the VM aborts without
// advancing the state.
func TestFinalize_FinalizeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().FinalizeLiveMigration(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	c := New()
	c.state = StateTransferCompleted
	c.origin = hcsschema.MigrationOriginSource
	c.vmController = vm

	if err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_STOP, nil); err == nil {
		t.Fatal("expected error, got nil")
	}
	if c.State() != StateTransferCompleted {
		t.Errorf("state = %s; want unchanged TransferCompleted", c.State())
	}
}

// TestFinalize_ResumeSucceeds verifies a resume finalizes the VM, resumes it,
// and advances to finalized on both origins.
func TestFinalize_ResumeSucceeds(t *testing.T) {
	cases := map[string]struct {
		origin        hcsschema.MigrationOrigin
		rebuildBridge bool
	}{
		"destination": {hcsschema.MigrationOriginDestination, false},
		"source":      {hcsschema.MigrationOriginSource, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			vm := mocks.NewMockvmController(ctrl)
			vm.EXPECT().FinalizeLiveMigration(gomock.Any(), gomock.Any()).Return(nil)
			vm.EXPECT().Resume(gomock.Any(), tc.rebuildBridge).Return(nil)

			c := New()
			c.state = StateTransferCompleted
			c.origin = tc.origin
			c.vmController = vm

			if err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_RESUME, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.State() != StateFinalized {
				t.Errorf("state = %s; want Finalized", c.State())
			}
		})
	}
}

// TestFinalize_StopSucceeds verifies a stop finalizes the VM and advances to
// finalized without resuming it.
func TestFinalize_StopSucceeds(t *testing.T) {
	for _, origin := range []hcsschema.MigrationOrigin{hcsschema.MigrationOriginSource, hcsschema.MigrationOriginDestination} {
		t.Run(string(origin), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			vm := mocks.NewMockvmController(ctrl)
			vm.EXPECT().FinalizeLiveMigration(gomock.Any(), gomock.Any()).Return(nil)

			c := New()
			c.state = StateTransferCompleted
			c.origin = origin
			c.vmController = vm

			if err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_STOP, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.State() != StateFinalized {
				t.Errorf("state = %s; want Finalized", c.State())
			}
		})
	}
}

// TestFinalize_ResumeError verifies a failure resuming the VM aborts without
// advancing the state.
func TestFinalize_ResumeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().FinalizeLiveMigration(gomock.Any(), gomock.Any()).Return(nil)
	vm.EXPECT().Resume(gomock.Any(), false).Return(errors.New("boom"))

	c := New()
	c.state = StateTransferCompleted
	c.origin = hcsschema.MigrationOriginDestination
	c.vmController = vm

	if err := c.Finalize(t.Context(), "sess-1", migration.FinalizeAction_FINALIZE_ACTION_RESUME, nil); err == nil {
		t.Fatal("expected error, got nil")
	}
	if c.State() != StateTransferCompleted {
		t.Errorf("state = %s; want unchanged TransferCompleted", c.State())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Cancel
// ─────────────────────────────────────────────────────────────────────────────

// TestCancel_RejectsSessionMismatch verifies a cancel for a different session is
// rejected.
func TestCancel_RejectsSessionMismatch(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"

	if err := c.Cancel(t.Context(), "other"); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestCancel_Idempotent verifies cancelling an already-cancelled session is a
// no-op.
func TestCancel_Idempotent(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"
	c.state = StateCancelled

	if err := c.Cancel(t.Context(), "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State() != StateCancelled {
		t.Errorf("state = %s; want Cancelled", c.State())
	}
}

// TestCancel_Success verifies cancelling an in-flight session moves it to
// cancelled.
func TestCancel_Success(t *testing.T) {
	c := New()
	c.sessionID = "sess-1"
	c.state = StateTransferCompleted

	if err := c.Cancel(t.Context(), "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State() != StateCancelled {
		t.Errorf("state = %s; want Cancelled", c.State())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Cleanup
// ─────────────────────────────────────────────────────────────────────────────

// TestCleanup_NoopWhenIdle verifies cleaning up an idle controller is a no-op.
func TestCleanup_NoopWhenIdle(t *testing.T) {
	c := New()

	if err := c.Cleanup(t.Context(), "sess-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCleanup_RejectsSessionMismatch verifies cleanup for a different session is
// rejected.
func TestCleanup_RejectsSessionMismatch(t *testing.T) {
	c := New()
	c.state = StateFinalized
	c.sessionID = "sess-1"

	if err := c.Cleanup(t.Context(), "other", nil); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestCleanup_RejectsNotFinalized verifies cleanup is only valid once the
// session has been finalized.
func TestCleanup_RejectsNotFinalized(t *testing.T) {
	c := New()
	c.state = StateTransferCompleted
	c.sessionID = "sess-1"

	if err := c.Cleanup(t.Context(), "sess-1", nil); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("expected ErrFailedPrecondition, got %v", err)
	}
}

// TestCleanup_Success verifies cleanup returns a finalized session to idle and
// clears its session-scoped state.
func TestCleanup_Success(t *testing.T) {
	c := New()
	c.state = StateFinalized
	c.sessionID = "sess-1"
	c.origin = hcsschema.MigrationOriginSource

	if err := c.Cleanup(t.Context(), "sess-1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State() != StateIdle {
		t.Errorf("state = %s; want Idle", c.State())
	}
	if c.sessionID != "" || c.vmController != nil || c.podControllers != nil {
		t.Errorf("session state not cleared: %+v", c)
	}
}

// TestCleanup_ClosesSocketAndNotifier verifies cleanup releases the transport
// socket and tears down the notifier before returning to idle.
func TestCleanup_ClosesSocketAndNotifier(t *testing.T) {
	if err := ensureWinsock(); err != nil {
		t.Skipf("winsock unavailable: %v", err)
	}
	sock, err := windows.Socket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_TCP)
	if err != nil {
		t.Fatalf("create socket: %v", err)
	}

	c := New()
	c.state = StateFinalized
	c.sessionID = "s"
	c.dupSocket = sock
	c.notifier = newTestNotifications(hcsschema.MigrationOriginSource)

	if err := c.Cleanup(context.Background(), "s", nil); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if c.State() != StateIdle {
		t.Errorf("state = %s; want Idle", c.State())
	}
	if c.dupSocket != 0 {
		t.Error("expected dupSocket to be cleared")
	}
	if c.notifier != nil {
		t.Error("expected notifier to be cleared")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// runTransfer
// ─────────────────────────────────────────────────────────────────────────────

// TestRunTransfer_Destination verifies the destination issues the destination
// start call followed by the transfer.
func TestRunTransfer_Destination(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().StartWithMigrationOptions(gomock.Any(), gomock.Any()).Return(nil)
	vm.EXPECT().StartLiveMigrationTransfer(gomock.Any(), gomock.Any()).Return(nil)

	c := New()
	c.sessionID = "s"
	c.origin = hcsschema.MigrationOriginDestination
	c.vmController = vm

	if err := c.runTransfer(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunTransfer_UnsupportedOrigin verifies an unknown origin is rejected.
func TestRunTransfer_UnsupportedOrigin(t *testing.T) {
	c := New()
	c.origin = hcsschema.MigrationOrigin("bogus")

	if err := c.runTransfer(t.Context()); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

// TestRunTransfer_StartError verifies a failure starting the migration surfaces.
func TestRunTransfer_StartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().StartLiveMigrationOnSource(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	c := New()
	c.origin = hcsschema.MigrationOriginSource
	c.vmController = vm

	if err := c.runTransfer(t.Context()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestRunTransfer_TransferError verifies a failure starting the transfer surfaces.
func TestRunTransfer_TransferError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().StartLiveMigrationOnSource(gomock.Any(), gomock.Any()).Return(nil)
	vm.EXPECT().StartLiveMigrationTransfer(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	c := New()
	c.origin = hcsschema.MigrationOriginSource
	c.vmController = vm

	if err := c.runTransfer(t.Context()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// failTransfer
// ─────────────────────────────────────────────────────────────────────────────

// TestFailTransfer_BroadcastsToSubscribers verifies a failed transfer marks the
// session failed and delivers a failure event to subscribers on either origin.
func TestFailTransfer_BroadcastsToSubscribers(t *testing.T) {
	for _, origin := range []hcsschema.MigrationOrigin{hcsschema.MigrationOriginSource, hcsschema.MigrationOriginDestination} {
		t.Run(string(origin), func(t *testing.T) {
			n := newTestNotifications(origin)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sub, err := n.subscribe(ctx)
			if err != nil {
				t.Fatalf("subscribe: %v", err)
			}

			c := New()
			c.origin = origin
			c.notifier = n

			c.failTransfer(context.Background(), errors.New("boom"))

			if c.State() != StateFailed {
				t.Errorf("state = %s; want Failed", c.State())
			}
			if got := recvWithin(t, sub, time.Second); got == nil {
				t.Fatal("expected a failure notification")
			}
		})
	}
}
