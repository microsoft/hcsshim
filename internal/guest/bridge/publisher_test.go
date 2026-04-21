//go:build linux

package bridge

import (
	"testing"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
)

func TestPublisher_NilBridgeQueues(t *testing.T) {
	p := newPublisher()
	p.publish(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "test"},
	})
	p.mu.Lock()
	if len(p.pending) != 1 {
		t.Fatalf("expected 1 queued notification, got %d", len(p.pending))
	}
	p.mu.Unlock()
}

func TestPublisher_SetBridgeDrains(t *testing.T) {
	p := newPublisher()
	// Queue two notifications while disconnected.
	p.publish(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "c1"},
	})
	p.publish(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "c2"},
	})

	p.mu.Lock()
	if len(p.pending) != 2 {
		t.Fatalf("expected 2 queued, got %d", len(p.pending))
	}
	p.mu.Unlock()

	// setBridge(nil) should not drain.
	p.setBridge(nil)
	p.mu.Lock()
	if len(p.pending) != 2 {
		t.Fatalf("expected 2 still queued after setBridge(nil), got %d", len(p.pending))
	}
	p.mu.Unlock()
}
