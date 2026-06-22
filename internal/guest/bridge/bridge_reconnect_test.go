//go:build linux

package bridge

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/sirupsen/logrus"
)

func TestBridge_NotificationQueuedWhenDisconnected(t *testing.T) {
	b := New(nil, false)
	// Bridge starts disconnected (connected == false).
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "c1"},
	})
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "c2"},
	})

	b.notifyMu.Lock()
	if len(b.pendingNotifications) != 2 {
		t.Fatalf("expected 2 queued, got %d", len(b.pendingNotifications))
	}
	b.notifyMu.Unlock()
}

func TestBridge_DrainOnReconnect(t *testing.T) {
	b := New(nil, false)

	// Queue notifications while disconnected.
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "c1"},
	})
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "c2"},
	})

	// Simulate what ListenAndServe does: create channels, start writer,
	// then drain.
	b.responseChan = make(chan bridgeResponse, 4)

	b.drainPendingNotifications()

	// Collect drained notifications.
	var ids []string
	for i := 0; i < 2; i++ {
		select {
		case resp := <-b.responseChan:
			n := resp.response.(*prot.ContainerNotification)
			ids = append(ids, n.ContainerID)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for notification %d", i)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 drained notifications, got %d", len(ids))
	}

	b.notifyMu.Lock()
	if len(b.pendingNotifications) != 0 {
		t.Fatalf("expected 0 pending after drain, got %d", len(b.pendingNotifications))
	}
	b.notifyMu.Unlock()
}

func TestBridge_DisconnectQueuesAfterDrain(t *testing.T) {
	b := New(nil, false)
	b.responseChan = make(chan bridgeResponse, 4)

	// Drain with nothing pending — just sets connected = true.
	b.drainPendingNotifications()

	// Send while connected — goes directly to responseChan.
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "live"},
	})

	select {
	case resp := <-b.responseChan:
		n := resp.response.(*prot.ContainerNotification)
		if n.ContainerID != "live" {
			t.Fatalf("expected 'live', got %q", n.ContainerID)
		}
	default:
		t.Fatal("expected notification on responseChan")
	}

	// Disconnect — future notifications should queue.
	b.disconnectNotifications()

	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "queued"},
	})

	b.notifyMu.Lock()
	if len(b.pendingNotifications) != 1 {
		t.Fatalf("expected 1 queued after disconnect, got %d", len(b.pendingNotifications))
	}
	b.notifyMu.Unlock()

	// Nothing should be on responseChan.
	select {
	case <-b.responseChan:
		t.Fatal("should not have received on responseChan after disconnect")
	default:
	}
}

func TestBridge_FullReconnectCycle(t *testing.T) {
	b := New(nil, false)

	// --- Iteration 1: simulate ListenAndServe ---
	r1, w1 := io.Pipe()
	b.responseChan = make(chan bridgeResponse, 4)
	b.quitChan = make(chan bool)

	go func() {
		for range b.responseChan {
		}
	}() // drain writer

	b.drainPendingNotifications()

	// Send a notification while connected.
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "iter1"},
	})

	// Simulate bridge drop — disconnect, close channels.
	b.disconnectNotifications()
	close(b.quitChan)
	close(b.responseChan)
	r1.Close()
	w1.Close()

	// --- Between iterations: container exits ---
	b.publishNotification(&prot.ContainerNotification{
		MessageBase: prot.MessageBase{ContainerID: "between"},
	})

	b.notifyMu.Lock()
	if len(b.pendingNotifications) != 1 || b.pendingNotifications[0].ContainerID != "between" {
		t.Fatalf("expected 'between' queued, got %v", b.pendingNotifications)
	}
	b.notifyMu.Unlock()

	// --- Iteration 2: reconnect ---
	b.responseChan = make(chan bridgeResponse, 4)
	b.quitChan = make(chan bool)

	b.drainPendingNotifications()

	select {
	case resp := <-b.responseChan:
		n := resp.response.(*prot.ContainerNotification)
		if n.ContainerID != "between" {
			t.Fatalf("expected 'between', got %q", n.ContainerID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for drained notification")
	}
}

// TestBridge_ListenAndServeResetsProtocolVersion verifies that ListenAndServe
// resets protVer to PvInvalid on entry, so a fresh NegotiateProtocol after a
// reconnect dispatches to the PvInvalid-registered handler instead of falling
// through to UnknownMessageHandler.
func TestBridge_ListenAndServeResetsProtocolVersion(t *testing.T) {
	logrus.SetOutput(io.Discard)

	lc := newLoopbackConnection()
	defer lc.close()

	// Mirror AssignHandlers: negotiate handler is registered only at PvInvalid.
	mux := NewBridgeMux()
	var dispatchedVer prot.ProtocolVersion
	mux.HandleFunc(prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid,
		func(r *Request) (RequestResponse, error) {
			dispatchedVer = r.Version
			return &prot.NegotiateProtocolResponse{
				MessageResponseBase: prot.MessageResponseBase{ActivityID: r.ActivityID},
				Version:             uint32(prot.PvV4),
			}, nil
		})

	// Simulate a Bridge whose protVer survived the previous connection.
	b := &Bridge{
		Handler: mux,
		protVer: prot.PvV4,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- b.ListenAndServe(lc.SRead(), lc.SWrite())
	}()
	defer func() {
		// Fire-and-forget quit; ListenAndServe's teardown handles the reader.
		b.quitChan <- true
		select {
		case err := <-serveErr:
			if err != nil {
				t.Errorf("ListenAndServe returned: %v", err)
			}
		default:
		}
	}()

	req := &prot.NegotiateProtocol{
		MessageBase:    prot.MessageBase{ActivityID: "00000000-0000-0000-0000-000000000002"},
		MinimumVersion: uint32(prot.PvV4),
		MaximumVersion: uint32(prot.PvV4),
	}
	if err := serverSend(lc.CWrite(), prot.ComputeSystemNegotiateProtocolV1, prot.SequenceID(1), req); err != nil {
		t.Fatalf("send NegotiateProtocol: %v", err)
	}

	header, body, err := serverRead(lc.CRead())
	if err != nil {
		t.Fatalf("read NegotiateProtocol response: %v", err)
	}
	if header.Type != prot.ComputeSystemResponseNegotiateProtocolV1 {
		t.Fatalf("unexpected response header type: %v", header.Type)
	}
	// Receiving the response guarantees the handler ran.
	if dispatchedVer != prot.PvInvalid {
		t.Fatalf("expected dispatch at PvInvalid, got %v", dispatchedVer)
	}

	// Must not be UnknownMessageHandler's HrNotImpl.
	resp := &prot.MessageResponseBase{}
	if err := json.Unmarshal(body, resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Result == int32(gcserr.HrNotImpl) {
		t.Fatalf("unexpected HrNotImpl response; body=%s", string(body))
	}
}
