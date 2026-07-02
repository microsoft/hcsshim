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
}

// newNotifications begins forwarding the VM's migration events to subscribers.
func newNotifications(vmController vmController, origin hcsschema.MigrationOrigin) (*notifications, error) {
	// Subscribe to the VM's migration events before any subscriber attaches.
	src, err := vmController.MigrationNotifications()
	if err != nil {
		return nil, fmt.Errorf("get migration notifications channel: %w", err)
	}

	notif := &notifications{
		subscribers: map[chan *migration.NotificationsResponse]struct{}{},
		startTime:   time.Now(),
		done:        make(chan struct{}),
		origin:      origin,
	}

	// Forward each VM migration event to subscribers until the session is torn down or
	// the source stops producing.
	go func() {
		for {
			select {
			// Session torn down: stop forwarding.
			case <-notif.done:
				return

			// Next VM migration event, or the source channel was closed.
			case info, ok := <-src:
				// Source closed: there is nothing left to forward.
				if !ok {
					return
				}

				// broadcast returns false once the notifier is closed.
				if !notif.broadcast(info) {
					return
				}
			}
		}
	}()

	return notif, nil
}

// Subscribe returns a stream of migration notifications for the session,
// beginning with the most recent one. The stream ends when ctx is canceled or
// the session is torn down.
func (c *Controller) Subscribe(ctx context.Context, sessionID string) (<-chan *migration.NotificationsResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reject callers without an active session or whose sessionID does not
	// match the active one.
	if c.sessionID != sessionID {
		return nil, fmt.Errorf("session id %q does not match active session %q: %w", sessionID, c.sessionID, errdefs.ErrInvalidArgument)
	}

	// Create the notifier on first use; it begins forwarding VM events as
	// soon as it exists, independent of whether any subscriber attaches.
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

// broadcast delivers info to every subscriber and caches it for replay,
// returning false once the notifier has been closed.
func (n *notifications) broadcast(info hcsschema.OperationSystemMigrationNotificationInfo) bool {
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
		Notification: migration.ToNotification(info, n.origin),
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
