//go:build linux

package bridge

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	log "github.com/Microsoft/hcsshim/internal/log"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/pkg/errors"
)

const emptyActivityID = "00000000-0000-0000-0000-000000000000"

// wraps `bridge.PublishNotification` to conform to `shim.Publish`, so that it can
// be used by containerd's `oom.Watcher` (github.com/containerd/containerd/pkg/oom)
type BridgePublisherFunc func(*prot.ContainerNotification)

var _ shim.Publisher = BridgePublisherFunc(func(_ *prot.ContainerNotification) {})

func (bp BridgePublisherFunc) Close() error {
	// noop
	return nil
}

func (bp BridgePublisherFunc) Publish(ctx context.Context, topic string, event events.Event) error {
	n, err := mapEventToNotification(topic, event)
	if err != nil {
		return errors.Wrapf(err, "could not handle topic %q and event %+v", topic, event)
	}

	log.G(ctx).WithField("event", fmt.Sprintf("%+v", event)).WithField("eventTopic", topic).Debug("publishing event over notification channel")

	bp(n)
	return nil
}

// may want to expand this to other events if they are sent over a shim.Publisher interface
func mapEventToNotification(topic string, event events.Event) (*prot.ContainerNotification, error) {
	switch topic {
	case runtime.TaskOOMEventTopic:
		e, ok := event.(*eventstypes.TaskOOM)
		if !ok {
			return nil, fmt.Errorf("unsupported event type %T", e)
		}

		notification := prot.ContainerNotification{
			MessageBase: prot.MessageBase{
				ActivityID:  emptyActivityID,
				ContainerID: e.ContainerID,
			},
			Type:       prot.NtOomEvent,
			Operation:  prot.AoNone,
			Result:     0,
			ResultInfo: "",
		}

		return &notification, nil
	default:
		return nil, fmt.Errorf("unsupported event topic %q", topic)
	}

}
