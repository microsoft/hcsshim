//go:build windows

package gcs

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/sirupsen/logrus"
)

func setMissingProcessExitGrace(t *testing.T, grace time.Duration) {
	t.Helper()
	old := missingProcessExitGrace
	missingProcessExitGrace = grace
	t.Cleanup(func() { missingProcessExitGrace = old })
}

// newMissingProcessTest returns a started bridge with a process whose
// WaitForProcess is pending, and the guest side of the bridge connection.
func newMissingProcessTest(t *testing.T, guestOS string) (*Process, io.ReadWriteCloser) {
	t.Helper()
	s, c := pipeConn()
	b := newBridge(s, nil, logrus.NewEntry(logrus.StandardLogger()))
	p := &Process{
		gc:       &GuestConnection{brdg: b, os: guestOS},
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
	t.Cleanup(func() { b.Close() })
	return p, c
}

// replySignalNotFound reads the SignalProcess request and answers hrNotFound.
func replySignalNotFound(t *testing.T, c io.ReadWriter) {
	t.Helper()
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
}

func sendWaitResponse(t *testing.T, c io.Writer, id int64, exitCode uint32) {
	t.Helper()
	resp, err := json.Marshal(&prot.ContainerWaitForProcessResponse{ExitCode: exitCode})
	if err != nil {
		t.Error(err)
		return
	}
	sendMessage(t, c, prot.MsgType(prot.RPCWaitForProcess)|prot.MsgTypeResponse, id, resp)
}

func signalMissing(t *testing.T, p *Process) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	signaled, err := p.Signal(ctx, nil)
	if err != nil {
		t.Fatalf("Signal() error = %v", err)
	}
	if signaled {
		t.Fatal("Signal() reported signaling a process the guest said was missing")
	}
}

func waitForExit(t *testing.T, p *Process) int {
	t.Helper()
	select {
	case <-p.waitCall.ch:
	case <-time.After(10 * time.Second):
		t.Fatal("pending WaitForProcess was not completed")
	}
	exitCode, err := p.ExitCode()
	if err != nil {
		t.Fatalf("ExitCode() error = %v", err)
	}
	return exitCode
}

// newStdioRelay returns a connected host-side relay channel and the guest end.
func newStdioRelay(t *testing.T) (*ioChannel, net.Conn) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ch := newIoChannel(l)
	guest, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	<-ch.ch
	t.Cleanup(func() {
		guest.Close()
		ch.Close()
	})
	return ch, guest
}

// assertRelayOpen checks that the stdio relay still carries guest output.
func assertRelayOpen(t *testing.T, host *ioChannel, guest net.Conn) {
	t.Helper()
	if _, err := guest.Write([]byte("x")); err != nil {
		t.Fatalf("guest write to stdio relay: %v", err)
	}
	if err := host.c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if _, err := io.ReadFull(host, b); err != nil {
		t.Fatalf("stdio relay was closed: %v", err)
	}
}

// The guest may answer the signal before sending the wait response of a
// process that just exited. The real exit code must be kept and the stdio
// relay left open.
func TestProcessSignalNotFoundKeepsRealExit(t *testing.T) {
	const grace = 20 * time.Millisecond
	setMissingProcessExitGrace(t, grace)
	p, c := newMissingProcessTest(t, "windows")
	var stdoutGuest net.Conn
	p.stdout, stdoutGuest = newStdioRelay(t)
	go func() {
		replySignalNotFound(t, c)
		sendWaitResponse(t, c, p.waitCall.id, 137)
		reflector(t, c, 0)
	}()

	signalMissing(t, p)
	if exitCode := waitForExit(t, p); exitCode != 137 {
		t.Fatalf("ExitCode() = %d, want real exit code 137", exitCode)
	}
	time.Sleep(5 * grace)
	assertRelayOpen(t, p.stdout, stdoutGuest)
}

// A Linux guest drains and closes the stdio relay itself before it answers
// the wait, so the host must not close the relay for it.
func TestProcessSignalNotFoundLinuxGuestKeepsStdio(t *testing.T) {
	const grace = 20 * time.Millisecond
	setMissingProcessExitGrace(t, grace)
	p, c := newMissingProcessTest(t, "linux")
	var stdoutGuest net.Conn
	p.stdout, stdoutGuest = newStdioRelay(t)
	signaled := make(chan struct{})
	go func() {
		replySignalNotFound(t, c)
		<-signaled
		sendWaitResponse(t, c, p.waitCall.id, 137)
		reflector(t, c, 0)
	}()

	signalMissing(t, p)
	time.Sleep(5 * grace)
	assertRelayOpen(t, p.stdout, stdoutGuest)
	close(signaled)
	if exitCode := waitForExit(t, p); exitCode != 137 {
		t.Fatalf("ExitCode() = %d, want real exit code 137", exitCode)
	}
}

// A Windows guest holds the wait response of a process until the host closes
// the process's stdio relay. Closing it must release the real exit code.
func TestProcessSignalNotFoundClosesStdioToReleaseExit(t *testing.T) {
	setMissingProcessExitGrace(t, 10*time.Millisecond)
	p, c := newMissingProcessTest(t, "windows")
	var stdoutGuest, stderrGuest net.Conn
	p.stdout, stdoutGuest = newStdioRelay(t)
	p.stderr, stderrGuest = newStdioRelay(t)
	const exitCode = 0xC000013A
	go func() {
		replySignalNotFound(t, c)
		for _, guest := range []net.Conn{stdoutGuest, stderrGuest} {
			if _, err := io.Copy(io.Discard, guest); err != nil {
				t.Error(err)
				return
			}
		}
		sendWaitResponse(t, c, p.waitCall.id, exitCode)
		reflector(t, c, 0)
	}()

	signalMissing(t, p)
	if got := waitForExit(t, p); got != exitCode {
		t.Fatalf("ExitCode() = %#x, want real exit code %#x", got, exitCode)
	}
}
