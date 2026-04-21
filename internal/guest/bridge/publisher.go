//go:build linux

package bridge

import (
	"sync"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/sirupsen/logrus"
)

// publisher provides a stable reference for container exit goroutines
// to send notifications through. It survives bridge recreation during
// live migration. When the bridge is nil, notifications are queued and
// drained when a new bridge is attached.
type publisher struct {
	mu      sync.Mutex
	b       *Bridge
	pending []*prot.ContainerNotification
}

// newPublisher creates a publisher with no bridge attached.
func newPublisher() *publisher {
	return &publisher{}
}

// setBridge attaches or detaches the current bridge. When a non-nil bridge
// is set, any queued notifications are drained through it immediately.
func (p *publisher) setBridge(b *Bridge) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.b = b
	if b != nil {
		for _, n := range p.pending {
			logrus.WithField("containerID", n.ContainerID).
				Info("draining queued container notification")
			b.PublishNotification(n)
		}
		p.pending = nil
	}
}

// publish sends a container notification to the current bridge.
// If no bridge is connected, the notification is queued for delivery
// when the next bridge is set.
func (p *publisher) publish(n *prot.ContainerNotification) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.b == nil {
		logrus.WithField("containerID", n.ContainerID).
			Warn("bridge not connected, queueing container notification")
		p.pending = append(p.pending, n)
		return
	}
	p.b.PublishNotification(n)
}
