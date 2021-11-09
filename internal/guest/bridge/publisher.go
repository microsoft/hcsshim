//go:build linux
// +build linux

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
	"github.com/sirupsen/logrus"
)

const emptyActivityID = "00000000-0000-0000-0000-000000000000"

var ErrBridgePublisherClosed = errors.New("bridge publisher closed")

// wraps `bridge.PublishNotification` to conform to `shim.Publish`, so that it can
// be used by containerd's `oom.Watcher` (github.com/containerd/containerd/pkg/oom)
type bridgePublisher struct {
	f func(*prot.ContainerNotification)
}

var _ shim.Publisher = bridgePublisher{}

func newBridgePublisher(publisherFunc func(*prot.ContainerNotification)) bridgePublisher {
	return bridgePublisher{publisherFunc}
}

func (bp bridgePublisher) Close() error {
	bp.f = nil
	return nil
}

func (bp bridgePublisher) Publish(ctx context.Context, topic string, event events.Event) error {
	if bp.f == nil {
		return ErrBridgePublisherClosed
	}
	n, err := mapEventToNotification(topic, event)
	if err != nil {
		return errors.Wrapf(err, "could not handle topic %q and event %+v", topic, event)
	}

	log.G(ctx).WithFields(logrus.Fields{
		"event": fmt.Sprintf("%+v", event),
		"eventTopic": topic,
	}).Debug("publishing event over notification channel")

	bp.f(n)
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
