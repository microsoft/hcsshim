//go:build windows && lcow

package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/Microsoft/hcsshim/internal/controller/migration/mocks"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/pkg/migration"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/errdefs"
)

// newTestNotifications builds a notifier without the source-forwarding
// goroutine so the fan-out logic can be driven directly via broadcast.
func newTestNotifications(origin hcsschema.MigrationOrigin) *notifications {
	return &notifications{
		subscribers: map[chan *migration.NotificationsResponse]struct{}{},
		startTime:   time.Now(),
		done:        make(chan struct{}),
		origin:      origin,
	}
}

func setupDoneInfo() hcsschema.OperationSystemMigrationNotificationInfo {
	return hcsschema.OperationSystemMigrationNotificationInfo{Event: hcsschema.MigrationEventSetupDone}
}

func setupDoneNotification(origin hcsschema.MigrationOrigin) *migration.Notification {
	return migration.ToMigrationNotification(setupDoneInfo(), origin)
}

// recvWithin returns the next notification or fails if none arrives in time.
func recvWithin(t *testing.T, ch <-chan *migration.NotificationsResponse, d time.Duration) *migration.NotificationsResponse {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(d):
		t.Fatal("timed out waiting for notification")
		return nil
	}
}

// waitChannelClosed drains ch until it is closed or fails on timeout.
func waitChannelClosed(t *testing.T, ch <-chan *migration.NotificationsResponse, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel not closed within timeout")
		}
	}
}

// TestNotificationsBroadcastDeliversToSubscriber verifies a broadcast reaches
// an attached subscriber with a populated response.
func TestNotificationsBroadcastDeliversToSubscriber(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if ok := n.broadcast(setupDoneNotification(n.origin)); !ok {
		t.Fatal("broadcast returned false on an open notifier")
	}

	got := recvWithin(t, sub, time.Second)
	if got.MessageID != 1 {
		t.Fatalf("messageID: got %d want 1", got.MessageID)
	}
	if got.Notification == nil || got.Notification.Phase != migration.Phase_PHASE_SETUP_DONE {
		t.Fatalf("unexpected notification: %+v", got.Notification)
	}
	if got.Notification.Origin != migration.Origin_ORIGIN_SOURCE {
		t.Fatalf("origin: got %s want %s", got.Notification.Origin, migration.Origin_ORIGIN_SOURCE)
	}
	if got.StartTime == nil || got.UpdateTime == nil {
		t.Fatal("expected StartTime and UpdateTime to be set")
	}
}

// TestControllerPublishTaskEvents verifies supported task events are delivered
// through the migration notification stream and unsupported events are ignored.
func TestControllerPublishTaskEvents(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	c := &Controller{notifier: n}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	c.PublishTaskEvents(&eventstypes.TaskOOM{ContainerID: "container"})
	select {
	case got := <-sub:
		t.Fatalf("unexpected notification for unsupported task event: %+v", got)
	default:
	}

	c.PublishTaskEvents(&eventstypes.TaskExit{
		ContainerID: "container",
		ID:          "exec",
		Pid:         42,
		ExitStatus:  137,
	})

	got := recvWithin(t, sub, time.Second)
	if got.Notification == nil ||
		got.Notification.Phase != migration.Phase_PHASE_TASK_EVENT ||
		got.Notification.State != migration.PhaseState_PHASE_STATE_TASK_EXIT {
		t.Fatalf("unexpected notification: %+v", got.Notification)
	}
	if details := got.Notification.GetTaskExit(); details == nil ||
		details.ContainerID != "container" ||
		details.ID != "exec" ||
		details.Pid != 42 ||
		details.ExitStatus != 137 {
		t.Fatalf("unexpected task exit details: %+v", details)
	}
}

// TestNotificationsBroadcastFansOutToAllSubscribers verifies every subscriber
// receives the same event.
func TestNotificationsBroadcastFansOutToAllSubscribers(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub1, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe sub1: %v", err)
	}
	sub2, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe sub2: %v", err)
	}

	n.broadcast(setupDoneNotification(n.origin))

	if r := recvWithin(t, sub1, time.Second); r.MessageID != 1 {
		t.Fatalf("sub1 messageID: got %d want 1", r.MessageID)
	}
	if r := recvWithin(t, sub2, time.Second); r.MessageID != 1 {
		t.Fatalf("sub2 messageID: got %d want 1", r.MessageID)
	}
}

// TestNotificationsBroadcastIncrementsMessageID verifies the per-stream
// counter increases monotonically across events.
func TestNotificationsBroadcastIncrementsMessageID(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	n.broadcast(setupDoneNotification(n.origin))
	n.broadcast(setupDoneNotification(n.origin))

	if r := recvWithin(t, sub, time.Second); r.MessageID != 1 {
		t.Fatalf("first messageID: got %d want 1", r.MessageID)
	}
	if r := recvWithin(t, sub, time.Second); r.MessageID != 2 {
		t.Fatalf("second messageID: got %d want 2", r.MessageID)
	}
}

// TestNotificationsSubscribeReplaysLatest verifies a late subscriber
// immediately receives the most recent event.
func TestNotificationsSubscribeReplaysLatest(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Broadcast before anyone subscribes.
	n.broadcast(setupDoneNotification(n.origin))

	sub, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if r := recvWithin(t, sub, time.Second); r.MessageID != 1 {
		t.Fatalf("replayed messageID: got %d want 1", r.MessageID)
	}
}

// TestNotificationsSubscribeAfterCloseFails verifies subscribing to a
// terminated notifier is rejected.
func TestNotificationsSubscribeAfterCloseFails(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	n.close()

	if _, err := n.subscribe(context.Background()); err == nil {
		t.Fatal("expected error subscribing to a closed notifier")
	}
}

// TestNotificationsBroadcastAfterCloseReturnsFalse verifies broadcast signals
// the forwarder to stop once the notifier is closed.
func TestNotificationsBroadcastAfterCloseReturnsFalse(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	n.close()

	if n.broadcast(setupDoneNotification(n.origin)) {
		t.Fatal("broadcast should return false after close")
	}
}

// TestNotificationsCloseClosesSubscribers verifies close ends every active
// subscriber stream.
func TestNotificationsCloseClosesSubscribers(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	n.close()
	waitChannelClosed(t, sub, time.Second)
}

// TestNotificationsCloseIsIdempotent verifies close can be called repeatedly.
func TestNotificationsCloseIsIdempotent(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	n.close()
	n.close()
}

// TestNotificationsSubscribeContextCancelDropsSubscriber verifies a canceled
// context removes and closes the subscriber, leaving other delivery intact.
func TestNotificationsSubscribeContextCancelDropsSubscriber(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())

	sub, err := n.subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	cancel()
	waitChannelClosed(t, sub, time.Second)

	// The notifier is still open and broadcasting must not panic on the
	// dropped subscriber.
	if ok := n.broadcast(setupDoneNotification(n.origin)); !ok {
		t.Fatal("broadcast on an open notifier returned false")
	}
}

// TestNotificationsBroadcastDoesNotBlockOnSlowSubscriber verifies a subscriber
// that never reads cannot stall the broadcaster.
func TestNotificationsBroadcastDoesNotBlockOnSlowSubscriber(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe but never drain the channel.
	if _, err := n.subscribe(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	done := make(chan struct{})
	go func() {
		// Broadcasting well past the buffer must not block; extras are dropped.
		for i := 0; i < subscriberBuffer*2; i++ {
			n.broadcast(setupDoneNotification(n.origin))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked on a slow subscriber")
	}
}

// TestNotificationsForwardVMNotificationsIsIdempotent verifies repeated calls
// start only one VM notification forwarder.
func TestNotificationsForwardVMNotificationsIsIdempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().MigrationNotifications().Return(make(chan hcsschema.OperationSystemMigrationNotificationInfo), nil).Times(1)

	n := newTestNotifications(hcsschema.MigrationOriginSource)
	defer n.close()

	if err := n.forwardVMNotifications(vm, hcsschema.MigrationOriginSource); err != nil {
		t.Fatalf("first forward: %v", err)
	}
	if err := n.forwardVMNotifications(vm, hcsschema.MigrationOriginSource); err != nil {
		t.Fatalf("second forward: %v", err)
	}
}

// TestNotificationsForwardVMNotificationsRetriesAfterError verifies a failed
// attachment does not prevent a later attempt from starting the forwarder.
func TestNotificationsForwardVMNotificationsRetriesAfterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	gomock.InOrder(
		vm.EXPECT().MigrationNotifications().Return(nil, errors.New("boom")),
		vm.EXPECT().MigrationNotifications().Return(make(chan hcsschema.OperationSystemMigrationNotificationInfo), nil),
	)

	n := newTestNotifications(hcsschema.MigrationOriginSource)
	defer n.close()

	if err := n.forwardVMNotifications(vm, hcsschema.MigrationOriginSource); err == nil {
		t.Fatal("expected first forward to fail")
	}
	if err := n.forwardVMNotifications(vm, hcsschema.MigrationOriginSource); err != nil {
		t.Fatalf("retry forward: %v", err)
	}
}

// TestControllerSubscribeBeforeSourceSetup verifies an early subscription
// receives task events immediately and VM events after source setup.
func TestControllerSubscribeBeforeSourceSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().InitializeLiveMigrationOnSource(gomock.Any(), gomock.Any()).Return(nil)
	src := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	vm.EXPECT().MigrationNotifications().Return(src, nil)

	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := c.Subscribe(ctx, "sess-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	c.PublishTaskEvents(&eventstypes.TaskExit{ContainerID: "container"})
	if got := recvWithin(t, sub, time.Second); got.Notification.GetTaskExit().GetContainerID() != "container" {
		t.Fatalf("unexpected task notification: %+v", got.Notification)
	}

	if err := c.PrepareSource(ctx, sourceOptions(vm)); err != nil {
		t.Fatalf("prepare source: %v", err)
	}
	defer c.notifier.close()

	src <- setupDoneInfo()
	if got := recvWithin(t, sub, time.Second); got.Notification.GetPhase() != migration.Phase_PHASE_SETUP_DONE {
		t.Fatalf("unexpected migration notification: %+v", got.Notification)
	}
}

// TestControllerSubscribeBeforeDestinationSetup verifies an early subscription
// begins forwarding VM events once the destination VM is prepared.
func TestControllerSubscribeBeforeDestinationSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := importVM(ctrl)
	vm.EXPECT().Import(gomock.Any(), gomock.Any()).Return(nil)
	vm.EXPECT().CreateVM(gomock.Any(), gomock.Any()).Return(nil)
	src := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	vm.EXPECT().MigrationNotifications().Return(src, nil)
	vm.EXPECT().Patch(gomock.Any()).Return(nil)

	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := c.Subscribe(ctx, "sess-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := c.ImportState(ctx, importOptions(vm, importEnvelope(t, validImportPayload()))); err != nil {
		t.Fatalf("import state: %v", err)
	}
	if err := c.PrepareDestination(ctx, "sess-1", nil); err != nil {
		t.Fatalf("prepare destination: %v", err)
	}
	defer c.notifier.close()

	src <- setupDoneInfo()
	if got := recvWithin(t, sub, time.Second); got.Notification.GetPhase() != migration.Phase_PHASE_SETUP_DONE {
		t.Fatalf("unexpected migration notification: %+v", got.Notification)
	}
}

// TestControllerSubscribeEmptySessionID verifies a session ID is required.
func TestControllerSubscribeEmptySessionID(t *testing.T) {
	c := New()
	if _, err := c.Subscribe(context.Background(), ""); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

// TestControllerSubscribeSessionMismatch verifies Subscribe rejects a
// sessionID that does not match the active one.
func TestControllerSubscribeSessionMismatch(t *testing.T) {
	c := &Controller{sessionID: "active"}
	if _, err := c.Subscribe(context.Background(), "other"); err == nil {
		t.Fatal("expected error on session mismatch")
	}
}

// TestControllerSubscribeReusesExistingNotifier verifies Subscribe attaches to
// an already-created notifier and delivers its events.
func TestControllerSubscribeReusesExistingNotifier(t *testing.T) {
	n := newTestNotifications(hcsschema.MigrationOriginSource)
	c := &Controller{sessionID: "s", notifier: n}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := c.Subscribe(ctx, "s")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	n.broadcast(setupDoneNotification(n.origin))
	if r := recvWithin(t, sub, time.Second); r.MessageID != 1 {
		t.Fatalf("messageID: got %d want 1", r.MessageID)
	}
}

// TestControllerSubscribeCreatesNotifier verifies the first Subscribe builds the
// notifier from the VM's event stream and forwards its events to the subscriber.
func TestControllerSubscribeCreatesNotifier(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)

	src := make(chan hcsschema.OperationSystemMigrationNotificationInfo, 1)
	var recv <-chan hcsschema.OperationSystemMigrationNotificationInfo = src
	vm.EXPECT().MigrationNotifications().Return(recv, nil)

	c := &Controller{sessionID: "s", origin: hcsschema.MigrationOriginSource, vmController: vm}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := c.Subscribe(ctx, "s")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer c.notifier.close()

	// An event on the VM stream is forwarded to the subscriber.
	src <- setupDoneInfo()
	if got := recvWithin(t, sub, time.Second); got == nil {
		t.Fatal("expected forwarded notification")
	}
}

// TestControllerSubscribeNotifierError verifies Subscribe surfaces a failure to
// obtain the VM's notification stream.
func TestControllerSubscribeNotifierError(t *testing.T) {
	ctrl := gomock.NewController(t)
	vm := mocks.NewMockvmController(ctrl)
	vm.EXPECT().MigrationNotifications().Return(nil, errors.New("boom"))

	c := &Controller{sessionID: "s", vmController: vm}
	if _, err := c.Subscribe(context.Background(), "s"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
