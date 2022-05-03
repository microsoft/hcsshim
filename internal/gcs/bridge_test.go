//go:build windows

package gcs

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	prot "github.com/Microsoft/hcsshim/internal/protocol/bridge"
	"github.com/sirupsen/logrus"
)

type stitched struct {
	io.ReadCloser
	io.WriteCloser
}

func (s *stitched) Close() error {
	s.ReadCloser.Close()
	s.WriteCloser.Close()
	return nil
}

func pipeConn() (*stitched, *stitched) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	return &stitched{r1, w2}, &stitched{r2, w1}
}

func sendMessage(t *testing.T, w io.Writer, mi prot.MessageIdentifier, id int64, msg []byte) {
	var h [16]byte
	binary.LittleEndian.PutUint32(h[:], uint32(mi))
	binary.LittleEndian.PutUint32(h[4:], uint32(len(msg)+16))
	binary.LittleEndian.PutUint64(h[8:], uint64(id))
	_, err := w.Write(h[:])
	if err != nil {
		t.Error(err)
		return
	}
	_, err = w.Write(msg)
	if err != nil {
		t.Error(err)
		return
	}
}

func reflector(t *testing.T, rw io.ReadWriteCloser, delay time.Duration) {
	defer rw.Close()
	for {
		id, typ, msg, err := readMessage(rw)
		if err != nil {
			if err != io.EOF {
				t.Error(err)
			}
			return
		}
		time.Sleep(delay) // delay is used to test timeouts (when non-zero)
		typ ^= prot.MessageIdentifier(prot.TypeResponse ^ prot.TypeRequest)
		sendMessage(t, rw, typ, id, msg)
	}
}

type testReq struct {
	prot.RequestBase
	X, Y int
}

type testResp struct {
	prot.ResponseBase
	X, Y int
}

func startReflectedBridge(t *testing.T, delay time.Duration) *bridge {
	s, c := pipeConn()
	b := newBridge(s, nil, logrus.NewEntry(logrus.StandardLogger()))
	b.Start()
	go reflector(t, c, delay)
	return b
}

func TestBridgeRPC(t *testing.T) {
	b := startReflectedBridge(t, 0)
	defer b.Close()
	req := testReq{X: 5}
	var resp testResp
	err := b.RPC(context.Background(), prot.RPCCreate, &req, &resp, false)
	if err != nil {
		t.Fatal(err)
	}
	if req.X != resp.X || req.Y != resp.Y {
		t.Fatalf("expected equal: %+v %+v", req, resp)
	}
}

func TestBridgeRPCResponseTimeout(t *testing.T) {
	b := startReflectedBridge(t, time.Minute)
	defer b.Close()
	b.Timeout = time.Millisecond * 100
	req := testReq{X: 5}
	var resp testResp
	err := b.RPC(context.Background(), prot.RPCCreate, &req, &resp, false)
	if err == nil || !strings.Contains(err.Error(), "bridge closed") {
		t.Fatalf("expected bridge disconnection, got %s", err)
	}
}

func TestBridgeRPCContextDone(t *testing.T) {
	b := startReflectedBridge(t, time.Minute)
	defer b.Close()
	b.Timeout = time.Millisecond * 250
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	req := testReq{X: 5}
	var resp testResp
	err := b.RPC(ctx, prot.RPCCreate, &req, &resp, true)
	if err != context.DeadlineExceeded {
		t.Fatalf("expected deadline exceeded, got %s", err)
	}
}

func TestBridgeRPCContextDoneNoCancel(t *testing.T) {
	b := startReflectedBridge(t, time.Minute)
	defer b.Close()
	b.Timeout = time.Millisecond * 250
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	req := testReq{X: 5}
	var resp testResp
	err := b.RPC(ctx, prot.RPCCreate, &req, &resp, false)
	if err == nil || !strings.Contains(err.Error(), "bridge closed") {
		t.Fatalf("expected bridge disconnection, got %s", err)
	}
}

func TestBridgeRPCBridgeClosed(t *testing.T) {
	b := startReflectedBridge(t, 0)
	eerr := errors.New("forcibly terminated")
	b.kill(eerr)
	err := b.RPC(context.Background(), prot.RPCCreate, nil, nil, false)
	if err != eerr {
		t.Fatal("unexpected: ", err)
	}
}

func sendJSON(t *testing.T, w io.Writer, mi prot.MessageIdentifier, id int64, msg interface{}) error {
	msgb, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	sendMessage(t, w, mi, id, msgb)
	return nil
}

func notifyThroughBridge(t *testing.T, mi prot.MessageIdentifier, msg interface{}, fn notifyFunc) error {
	s, c := pipeConn()
	b := newBridge(s, fn, logrus.NewEntry(logrus.StandardLogger()))
	b.Start()
	err := sendJSON(t, c, mi, 0, msg)
	if err != nil {
		b.Close()
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return b.Close()
}

func TestBridgeNotify(t *testing.T) {
	ntf := &prot.ContainerNotification{Operation: "testing"}
	recvd := false
	err := notifyThroughBridge(t, prot.NewIdentifier(prot.TypeNotify, prot.NotifyContainer), ntf, func(nntf *prot.ContainerNotification) error {
		if !reflect.DeepEqual(ntf, nntf) {
			t.Errorf("%+v != %+v", ntf, nntf)
		}
		recvd = true
		return nil
	})
	if err != nil {
		t.Error("notify failed: ", err)
	}
	if !recvd {
		t.Error("did not receive notification")
	}
}

func TestBridgeNotifyFailure(t *testing.T) {
	ntf := &prot.ContainerNotification{Operation: "testing"}
	errMsg := "notify should have failed"
	err := notifyThroughBridge(t, prot.NewIdentifier(prot.TypeNotify, prot.NotifyContainer), ntf, func(nntf *prot.ContainerNotification) error {
		return errors.New(errMsg)
	})
	if err == nil || !strings.Contains(err.Error(), errMsg) {
		t.Error("unexpected result: ", err)
	}
}
