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

	if ok := n.broadcast(setupDoneInfo()); !ok {
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

	n.broadcast(setupDoneInfo())

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

	n.broadcast(setupDoneInfo())
	n.broadcast(setupDoneInfo())

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
	n.broadcast(setupDoneInfo())

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

	if n.broadcast(setupDoneInfo()) {
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
	if ok := n.broadcast(setupDoneInfo()); !ok {
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
			n.broadcast(setupDoneInfo())
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked on a slow subscriber")
	}
}

// TestControllerSubscribeNoActiveSession verifies Subscribe is rejected when
// no session is active.
func TestControllerSubscribeNoActiveSession(t *testing.T) {
	c := &Controller{}
	if _, err := c.Subscribe(context.Background(), "any"); err == nil {
		t.Fatal("expected error when no session is active")
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

	n.broadcast(setupDoneInfo())
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
