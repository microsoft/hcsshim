//go:build windows

package gcs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/sirupsen/logrus"
)

func TestProcessSignalNotFoundCompletesPendingWait(t *testing.T) {
	s, c := pipeConn()
	b := newBridge(s, nil, logrus.NewEntry(logrus.StandardLogger()))
	p := &Process{
		gc:       &GuestConnection{brdg: b},
		cid:      t.Name(),
		id:       1068,
		waitResp: &prot.ContainerWaitForProcessResponse{},
	}
	p.waitCall = &rpc{
		ch:   make(chan struct{}),
		id:   42,
		proc: prot.RPCWaitForProcess,
		resp: p.waitResp,
	}
	b.rpcs[p.waitCall.id] = p.waitCall
	b.nextID = p.waitCall.id + 1
	b.Start()
	defer b.Close()

	sendLateWait := make(chan struct{})
	go func() {
		signalID, signalType, _, err := readMessage(c)
		if err != nil {
			t.Error(err)
			return
		}
		if got := signalType &^ prot.MsgTypeRequest; got != prot.MsgType(prot.RPCSignalProcess) {
			t.Errorf("request type = %s, want SignalProcess", signalType)
			return
		}
		result := uint32(hrNotFound)
		resp, err := json.Marshal(&prot.ResponseBase{
			Result:       int32(result),
			ErrorMessage: "Element not found.",
		})
		if err != nil {
			t.Error(err)
			return
		}
		sendMessage(t, c, signalType^prot.MsgTypeRequest^prot.MsgTypeResponse, signalID, resp)

		<-sendLateWait
		lateWaitResp, err := json.Marshal(&prot.ContainerWaitForProcessResponse{ExitCode: 1})
		if err != nil {
			t.Error(err)
			return
		}
		sendMessage(t, c, prot.MsgType(prot.RPCWaitForProcess)|prot.MsgTypeResponse, p.waitCall.id, lateWaitResp)
		reflector(t, c, 0)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	signaled, err := p.Signal(ctx, nil)
	if err != nil {
		t.Fatalf("Signal() error = %v", err)
	}
	if signaled {
		t.Fatal("Signal() reported signaling a process the guest said was missing")
	}
	if !p.waitCall.Done() {
		t.Fatal("pending WaitForProcess was not completed")
	}
	if err := p.waitCall.Err(); err != nil {
		t.Fatalf("force-completed WaitForProcess error = %v", err)
	}

	close(sendLateWait)
	var resp testResp
	req := testReq{X: 7}
	if err := b.RPC(ctx, prot.RPCCreate, &req, &resp, false); err != nil {
		t.Fatalf("bridge should survive the late WaitForProcess response: %v", err)
	}
	if resp.X != req.X {
		t.Fatalf("response X = %d, want %d", resp.X, req.X)
	}
}
