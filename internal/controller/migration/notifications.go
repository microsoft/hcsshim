//go:build windows && lcow

package migration

import (
	"context"
	"fmt"
	"sync"
	"time"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/pkg/migration"

	"github.com/containerd/errdefs"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// subscriberBuffer caps how many notifications a subscriber may queue before
// slow readers start dropping updates.
const subscriberBuffer = 64

// notifications fans migration events out to every subscriber and replays
// the latest event to late subscribers, all sharing one messageID sequence.
type notifications struct {
	// mu guards the mutable fields below.
	mu sync.RWMutex

	// subscribers is the set of active streams each event is delivered to.
	subscribers map[chan *migration.NotificationsResponse]struct{}

	// lastResponse is the most recent event, replayed to new subscribers.
	lastResponse *migration.NotificationsResponse

	// messageID is the monotonically increasing sequence number on each event.
	messageID uint32

	// startTime is when this notifier was created, reported as StartTime on every event.
	startTime time.Time

	// done is closed by close to end the session and all subscriber streams.
	done chan struct{}

	// origin identifies this host's role (source or destination) in the migration.
	origin hcsschema.MigrationOrigin

	// vmNotificationsStarted indicates that VM migration events are being forwarded.
	vmNotificationsStarted bool
}

// newNotifications creates a notifier and starts VM event forwarding when the
// VM controller is available.
func newNotifications(vmController vmController, origin hcsschema.MigrationOrigin) (*notifications, error) {
	notif := &notifications{
		subscribers: map[chan *migration.NotificationsResponse]struct{}{},
		startTime:   time.Now(),
		done:        make(chan struct{}),
	}

	// Before migration setup, the notifier carries task events only. If the VM
	// is already available, begin forwarding its migration events immediately.
	if vmController != nil {
		if err := notif.forwardVMNotifications(vmController, origin); err != nil {
			return nil, err
		}
	}

	return notif, nil
}

// forwardVMNotifications begins forwarding the VM's migration events to subscribers.
func (n *notifications) forwardVMNotifications(vmController vmController, origin hcsschema.MigrationOrigin) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Repeated forwarding attempts are no-ops once it has started.
	if n.vmNotificationsStarted {
		return nil
	}

	// Obtain the VM event stream from the VM controller.
	src, err := vmController.MigrationNotifications()
	if err != nil {
		return fmt.Errorf("get migration notifications channel: %w", err)
	}

	// Record the stream origin and prevent another forwarding goroutine.
	n.origin = origin
	n.vmNotificationsStarted = true

	// Forward each VM migration event to subscribers until the session is torn down or
	// the source stops producing.
	go func() {
		for {
			select {
			// Session torn down: stop forwarding.
			case <-n.done:
				return

			// Next VM migration event, or the source channel was closed.
			case info, ok := <-src:
				// Source closed: there is nothing left to forward.
				if !ok {
					return
				}

				// broadcast returns false once the notifier is closed.
				if !n.broadcast(migration.ToMigrationNotification(info, origin)) {
					return
				}
			}
		}
	}()

	return nil
}

// Subscribe returns a stream of migration notifications for the session,
// beginning with the most recent one. The stream ends when ctx is canceled or
// the session is torn down.
func (c *Controller) Subscribe(ctx context.Context, sessionID string) (<-chan *migration.NotificationsResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// User must supply a valid session while Subscribing for notifications.
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required: %w", errdefs.ErrInvalidArgument)
	}

	// The first migration call reserves the session ID. Every later call must
	// use the same value, regardless of whether Subscribe or setup ran first.
	if c.sessionID != "" && c.sessionID != sessionID {
		return nil, fmt.Errorf("session id %q does not match current session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	c.sessionID = sessionID

	// Create the notifier on first use. Before migration setup it carries task
	// events only; once the VM is available it also forwards VM events.
	if c.notifier == nil {
		notifier, err := newNotifications(c.vmController, c.origin)
		if err != nil {
			return nil, err
		}

		c.notifier = notifier
	}

	log.G(ctx).Debug("migration notification subscriber attached")
	return c.notifier.subscribe(ctx)
}

// PublishTaskEvents forwards supported containerd task events to migration notification subscribers.
func (c *Controller) PublishTaskEvents(event interface{}) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Task events cannot be delivered before the notifier is initialized.
	// After finalization, task exits are expected during source teardown and
	// must not be surfaced as migration notifications.
	if c.notifier == nil || c.state == StateFinalized {
		return
	}

	notification, ok := migration.ToTaskEventNotification(event, c.origin)
	if !ok {
		return
	}
	c.notifier.broadcast(notification)
}

// subscribe returns a channel that first replays the latest notification, then
// delivers every later one until ctx is canceled or the notifier closes.
func (n *notifications) subscribe(ctx context.Context) (<-chan *migration.NotificationsResponse, error) {
	subscriber := make(chan *migration.NotificationsResponse, subscriberBuffer)

	n.mu.Lock()
	defer n.mu.Unlock()

	// Session already terminated: reject the subscription.
	select {
	case <-n.done:
		return nil, fmt.Errorf("migration session already terminated: %w", errdefs.ErrFailedPrecondition)
	default:
	}

	// Replay the latest event so a late subscriber has immediate context;
	// the buffered channel keeps this send non-blocking.
	if n.lastResponse != nil {
		subscriber <- n.lastResponse
	}
	n.subscribers[subscriber] = struct{}{}

	// Drop the subscriber once its context ends or the notifier closes.
	go func() {
		select {
		case <-ctx.Done():
		case <-n.done:
			return
		}

		n.mu.Lock()
		defer n.mu.Unlock()

		// Skip if broadcast or close already removed this subscriber, to
		// avoid a double close.
		if _, ok := n.subscribers[subscriber]; ok {
			delete(n.subscribers, subscriber)
			close(subscriber)
		}
	}()

	return subscriber, nil
}

// broadcast delivers notification to every subscriber and caches it for replay,
// returning false once the notifier has been closed.
func (n *notifications) broadcast(notification *migration.Notification) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Notifier closed: drop the event and signal the forwarder to stop.
	select {
	case <-n.done:
		return false
	default:
	}

	// Stamp the next sequence number and cache the event for replay.
	n.messageID++
	n.lastResponse = &migration.NotificationsResponse{
		MessageID:    n.messageID,
		Notification: notification,
		StartTime:    timestamppb.New(n.startTime),
		UpdateTime:   timestamppb.Now(),
	}

	// Non-blocking send so one slow subscriber cannot stall the others.
	for subscriber := range n.subscribers {
		select {
		case subscriber <- n.lastResponse:
		default:
		}
	}

	return true
}

// close stops forwarding events and closes every subscriber channel. Safe to
// call more than once.
func (n *notifications) close() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Already closed: nothing to do.
	select {
	case <-n.done:
		return
	default:
	}

	// Signal the forwarder to stop, then close every subscriber stream.
	close(n.done)
	for subscriber := range n.subscribers {
		close(subscriber)
		delete(n.subscribers, subscriber)
	}
}
