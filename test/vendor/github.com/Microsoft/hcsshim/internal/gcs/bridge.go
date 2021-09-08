package gcs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

const (
	hdrSize    = 16
	hdrOffType = 0
	hdrOffSize = 4
	hdrOffID   = 8

	// maxMsgSize is the maximum size of an incoming message. This is not
	// enforced by the guest today but some maximum must be set to avoid
	// unbounded allocations.
	maxMsgSize = 0x10000
)

type requestMessage interface {
	Base() *requestBase
}

type responseMessage interface {
	Base() *responseBase
}

// rpc represents an outstanding rpc request to the guest
type rpc struct {
	proc    rpcProc
	id      int64
	req     requestMessage
	resp    responseMessage
	brdgErr error // error encountered when sending the request or unmarshaling the result
	ch      chan struct{}
}

// bridge represents a communcations bridge with the guest. It handles the
// transport layer but (mostly) does not parse or construct the message payload.
type bridge struct {
	// Timeout is the time a synchronous RPC must respond within.
	Timeout time.Duration

	mu      sync.Mutex
	nextID  int64
	rpcs    map[int64]*rpc
	conn    io.ReadWriteCloser
	rpcCh   chan *rpc
	notify  notifyFunc
	closed  bool
	log     *logrus.Entry
	brdgErr error
	waitCh  chan struct{}
}

var (
	errBridgeClosed = errors.New("bridge closed")
)

const (
	// bridgeFailureTimeout is the default value for bridge.Timeout
	bridgeFailureTimeout = time.Minute * 5
)

type notifyFunc func(*containerNotification) error

// newBridge returns a bridge on `conn`. It calls `notify` when a
// notification message arrives from the guest. It logs transport errors and
// traces using `log`.
func newBridge(conn io.ReadWriteCloser, notify notifyFunc, log *logrus.Entry) *bridge {
	return &bridge{
		conn:    conn,
		rpcs:    make(map[int64]*rpc),
		rpcCh:   make(chan *rpc),
		waitCh:  make(chan struct{}),
		notify:  notify,
		log:     log,
		Timeout: bridgeFailureTimeout,
	}
}

// Start begins the bridge send and receive goroutines.
func (brdg *bridge) Start() {
	go brdg.recvLoopRoutine()
	go brdg.sendLoop()
}

// kill terminates the bridge, closing the connection and causing all new and
// existing RPCs to fail.
func (brdg *bridge) kill(err error) {
	brdg.mu.Lock()
	if brdg.closed {
		brdg.mu.Unlock()
		if err != nil {
			brdg.log.WithError(err).Warn("bridge error, already terminated")
		}
		return
	}
	brdg.closed = true
	brdg.mu.Unlock()
	brdg.brdgErr = err
	if err != nil {
		brdg.log.WithError(err).Error("bridge forcibly terminating")
	} else {
		brdg.log.Debug("bridge terminating")
	}
	brdg.conn.Close()
	close(brdg.waitCh)
}

// Close closes the bridge. Calling RPC or AsyncRPC after calling Close will
// panic.
func (brdg *bridge) Close() error {
	brdg.kill(nil)
	return brdg.brdgErr
}

// Wait waits for the bridge connection to terminate and returns the bridge
// error, if any.
func (brdg *bridge) Wait() error {
	<-brdg.waitCh
	return brdg.brdgErr
}

// AsyncRPC sends an RPC request to the guest but does not wait for a response.
// If the message cannot be sent before the context is done, then an error is
// returned.
func (brdg *bridge) AsyncRPC(ctx context.Context, proc rpcProc, req requestMessage, resp responseMessage) (*rpc, error) {
	call := &rpc{
		ch:   make(chan struct{}),
		proc: proc,
		req:  req,
		resp: resp,
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Send the request.
	select {
	case brdg.rpcCh <- call:
		return call, nil
	case <-brdg.waitCh:
		err := brdg.brdgErr
		if err == nil {
			err = errBridgeClosed
		}
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (call *rpc) complete(err error) {
	call.brdgErr = err
	close(call.ch)
}

type rpcError struct {
	result  int32
	message string
}

func (err *rpcError) Error() string {
	msg := err.message
	if msg == "" {
		msg = windows.Errno(err.result).Error()
	}
	return "guest RPC failure: " + msg
}

// IsNotExist is a helper function to determine if the inner rpc error is Not Exist
func IsNotExist(err error) bool {
	switch rerr := err.(type) {
	case *rpcError:
		return uint32(rerr.result) == hrComputeSystemDoesNotExist
	}
	return false
}

// Err returns the RPC's result. This may be a transport error or an error from
// the message response.
func (call *rpc) Err() error {
	if call.brdgErr != nil {
		return call.brdgErr
	}
	resp := call.resp.Base()
	if resp.Result == 0 {
		return nil
	}
	return &rpcError{result: resp.Result, message: resp.ErrorMessage}
}

// Done returns whether the RPC has completed.
func (call *rpc) Done() bool {
	select {
	case <-call.ch:
		return true
	default:
		return false
	}
}

// Wait waits for the RPC to complete.
func (call *rpc) Wait() {
	<-call.ch
}

// RPC issues a synchronous RPC request. Returns immediately if the context
// becomes done and the message is not sent.
//
// If allowCancel is set and the context becomes done, returns an error without
// waiting for a response. Avoid this on messages that are not idempotent or
// otherwise safe to ignore the response of.
func (brdg *bridge) RPC(ctx context.Context, proc rpcProc, req requestMessage, resp responseMessage, allowCancel bool) error {
	call, err := brdg.AsyncRPC(ctx, proc, req, resp)
	if err != nil {
		return err
	}
	var ctxDone <-chan struct{}
	if allowCancel {
		// This message can be safely cancelled by ignoring the response.
		ctxDone = ctx.Done()
	}
	t := time.NewTimer(brdg.Timeout)
	defer t.Stop()
	select {
	case <-call.ch:
		return call.Err()
	case <-ctxDone:
		brdg.log.WithField("reason", ctx.Err()).Warn("ignoring response to bridge message")
		return ctx.Err()
	case <-t.C:
		brdg.kill(errors.New("message timeout"))
		<-call.ch
		return call.Err()
	}
}

func (brdg *bridge) recvLoopRoutine() {
	brdg.kill(brdg.recvLoop())
	// Fail any remaining RPCs.
	brdg.mu.Lock()
	rpcs := brdg.rpcs
	brdg.rpcs = nil
	brdg.mu.Unlock()
	for _, call := range rpcs {
		call.complete(errBridgeClosed)
	}
}

func readMessage(r io.Reader) (int64, msgType, []byte, error) {
	var h [hdrSize]byte
	_, err := io.ReadFull(r, h[:])
	if err != nil {
		return 0, 0, nil, err
	}
	typ := msgType(binary.LittleEndian.Uint32(h[hdrOffType:]))
	n := binary.LittleEndian.Uint32(h[hdrOffSize:])
	id := int64(binary.LittleEndian.Uint64(h[hdrOffID:]))
	if n < hdrSize || n > maxMsgSize {
		return 0, 0, nil, fmt.Errorf("invalid message size %d", n)
	}
	n -= hdrSize
	b := make([]byte, n)
	_, err = io.ReadFull(r, b)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return 0, 0, nil, err
	}
	return id, typ, b, nil
}

func isLocalDisconnectError(err error) bool {
	if o, ok := err.(*net.OpError); ok {
		if s, ok := o.Err.(*os.SyscallError); ok {
			return s.Err == syscall.WSAECONNABORTED
		}
	}
	return false
}

func (brdg *bridge) recvLoop() error {
	br := bufio.NewReader(brdg.conn)
	for {
		id, typ, b, err := readMessage(br)
		if err != nil {
			if err == io.EOF || isLocalDisconnectError(err) {
				return nil
			}
			return fmt.Errorf("bridge read failed: %s", err)
		}
		brdg.log.WithFields(logrus.Fields{
			"payload":    string(b),
			"type":       typ,
			"message-id": id}).Debug("bridge receive")
		switch typ & msgTypeMask {
		case msgTypeResponse:
			// Find the request associated with this response.
			brdg.mu.Lock()
			call := brdg.rpcs[id]
			delete(brdg.rpcs, id)
			brdg.mu.Unlock()
			if call == nil {
				return fmt.Errorf("bridge received unknown rpc response for id %d, type %s", id, typ)
			}
			err := json.Unmarshal(b, call.resp)
			if err != nil {
				err = fmt.Errorf("bridge response unmarshal failed: %s", err)
			} else if resp := call.resp.Base(); resp.Result != 0 {
				for _, rec := range resp.ErrorRecords {
					brdg.log.WithFields(logrus.Fields{
						"message-id":     id,
						"result":         rec.Result,
						"result-message": windows.Errno(rec.Result).Error(),
						"error-message":  rec.Message,
						"stack":          rec.StackTrace,
						"module":         rec.ModuleName,
						"file":           rec.FileName,
						"line":           rec.Line,
						"function":       rec.FunctionName,
					}).Error("bridge RPC error record")
				}
			}
			call.complete(err)
			if err != nil {
				return err
			}

		case msgTypeNotify:
			if typ != notifyContainer|msgTypeNotify {
				return fmt.Errorf("bridge received unknown unknown notification message %s", typ)
			}
			var ntf containerNotification
			ntf.ResultInfo.Value = &json.RawMessage{}
			err := json.Unmarshal(b, &ntf)
			if err != nil {
				return fmt.Errorf("bridge response unmarshal failed: %s", err)
			}
			err = brdg.notify(&ntf)
			if err != nil {
				return fmt.Errorf("bridge notification failed: %s", err)
			}
		default:
			return fmt.Errorf("bridge received unknown unknown message type %s", typ)
		}
	}
}

func (brdg *bridge) sendLoop() {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for {
		select {
		case <-brdg.waitCh:
			// The bridge has been killed.
			return
		case call := <-brdg.rpcCh:
			err := brdg.sendRPC(&buf, enc, call)
			if err != nil {
				brdg.kill(err)
				return
			}
		}
	}
}

func (brdg *bridge) writeMessage(buf *bytes.Buffer, enc *json.Encoder, typ msgType, id int64, req interface{}) error {
	// Prepare the buffer with the message.
	var h [hdrSize]byte
	binary.LittleEndian.PutUint32(h[hdrOffType:], uint32(typ))
	binary.LittleEndian.PutUint64(h[hdrOffID:], uint64(id))
	buf.Write(h[:])
	err := enc.Encode(req)
	if err != nil {
		return fmt.Errorf("bridge encode: %s", err)
	}
	// Update the message header with the size.
	binary.LittleEndian.PutUint32(buf.Bytes()[hdrOffSize:], uint32(buf.Len()))
	// Write the message.
	brdg.log.WithFields(logrus.Fields{
		"payload":    string(buf.Bytes()[hdrSize:]),
		"type":       typ,
		"message-id": id}).Debug("bridge send")
	_, err = buf.WriteTo(brdg.conn)
	if err != nil {
		return fmt.Errorf("bridge write: %s", err)
	}
	return nil
}

func (brdg *bridge) sendRPC(buf *bytes.Buffer, enc *json.Encoder, call *rpc) error {
	// Prepare the message for the response.
	brdg.mu.Lock()
	if brdg.rpcs == nil {
		brdg.mu.Unlock()
		call.complete(errBridgeClosed)
		return nil
	}
	id := brdg.nextID
	call.id = id
	brdg.rpcs[id] = call
	brdg.nextID++
	brdg.mu.Unlock()
	typ := msgType(call.proc) | msgTypeRequest
	err := brdg.writeMessage(buf, enc, typ, id, call.req)
	if err != nil {
		// Try to reclaim this request and fail it.
		brdg.mu.Lock()
		if brdg.rpcs[id] == nil {
			call = nil
		}
		delete(brdg.rpcs, id)
		brdg.mu.Unlock()
		if call != nil {
			call.complete(err)
		} else {
			brdg.log.WithError(err).Error("bridge write failed but call is already complete")
		}
		return err
	}
	return nil
}
