//go:build windows

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
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
)

type requestMessage interface {
	Base() *prot.RequestBase
}

type responseMessage interface {
	Base() *prot.ResponseBase
}

// rpc represents an outstanding rpc request to the guest
type rpc struct {
	proc    prot.RPCProc
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

var errBridgeClosed = fmt.Errorf("bridge closed: %w", net.ErrClosed)

const (
	// bridgeFailureTimeout is the default value for bridge.Timeout
	bridgeFailureTimeout = time.Minute * 5
)

type notifyFunc func(*prot.ContainerNotification) error

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
func (brdg *bridge) AsyncRPC(ctx context.Context, proc prot.RPCProc, req requestMessage, resp responseMessage) (*rpc, error) {
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

func (err *rpcError) Unwrap() error {
	return windows.Errno(err.result)
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
func (brdg *bridge) RPC(ctx context.Context, proc prot.RPCProc, req requestMessage, resp responseMessage, allowCancel bool) error {
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

func readMessage(r io.Reader) (int64, prot.MsgType, []byte, error) {
	_, span := oc.StartSpan(context.Background(), "bridge receive read message", oc.WithClientSpanKind)
	defer span.End()

	var h [prot.HdrSize]byte
	_, err := io.ReadFull(r, h[:])
	if err != nil {
		return 0, 0, nil, err
	}
	typ := prot.MsgType(binary.LittleEndian.Uint32(h[prot.HdrOffType:]))
	n := binary.LittleEndian.Uint32(h[prot.HdrOffSize:])
	id := int64(binary.LittleEndian.Uint64(h[prot.HdrOffID:]))
	span.AddAttributes(
		trace.StringAttribute("type", typ.String()),
		trace.Int64Attribute("message-id", id))

	if n < prot.HdrSize || n > prot.MaxMsgSize {
		return 0, 0, nil, fmt.Errorf("invalid message size %d", n)
	}
	n -= prot.HdrSize
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
	return errors.Is(err, windows.WSAECONNABORTED)
}

func (brdg *bridge) recvLoop() error {
	br := bufio.NewReader(brdg.conn)
	for {
		id, typ, b, err := readMessage(br)
		if err != nil {
			if err == io.EOF || isLocalDisconnectError(err) { //nolint:errorlint
				return nil
			}
			return fmt.Errorf("bridge read failed: %w", err)
		}
		brdg.log.WithFields(logrus.Fields{
			"payload":    string(b),
			"type":       typ.String(),
			"message-id": id}).Trace("bridge receive")

		switch typ & prot.MsgTypeMask {
		case prot.MsgTypeResponse:
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
				err = fmt.Errorf("bridge response unmarshal failed: %w", err)
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

		case prot.MsgTypeNotify:
			if typ != prot.NotifyContainer|prot.MsgTypeNotify {
				return fmt.Errorf("bridge received unknown unknown notification message %s", typ)
			}
			var ntf prot.ContainerNotification
			ntf.ResultInfo.Value = &json.RawMessage{}
			err := json.Unmarshal(b, &ntf)
			if err != nil {
				return fmt.Errorf("bridge response unmarshal failed: %w", err)
			}
			err = brdg.notify(&ntf)
			if err != nil {
				return fmt.Errorf("bridge notification failed: %w", err)
			}
		default:
			return fmt.Errorf("bridge received unknown unknown message type %s", typ)
		}
	}
}

func (brdg *bridge) sendLoop() {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
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

func (brdg *bridge) writeMessage(buf *bytes.Buffer, enc *json.Encoder, typ prot.MsgType, id int64, req interface{}) error {
	var err error
	_, span := oc.StartSpan(context.Background(), "bridge send", oc.WithClientSpanKind)
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("type", typ.String()),
		trace.Int64Attribute("message-id", id))

	// Prepare the buffer with the message.
	var h [prot.HdrSize]byte
	binary.LittleEndian.PutUint32(h[prot.HdrOffType:], uint32(typ))
	binary.LittleEndian.PutUint64(h[prot.HdrOffID:], uint64(id))
	buf.Write(h[:])
	err = enc.Encode(req)
	if err != nil {
		return fmt.Errorf("bridge encode: %w", err)
	}
	// Update the message header with the size.
	binary.LittleEndian.PutUint32(buf.Bytes()[prot.HdrOffSize:], uint32(buf.Len()))

	if brdg.log.Logger.GetLevel() > logrus.DebugLevel {
		b := buf.Bytes()[prot.HdrSize:]
		switch typ {
		// container environment vars are in rpCreate for linux; rpcExecuteProcess for windows
		case prot.MsgType(prot.RPCCreate) | prot.MsgTypeRequest:
			b, err = log.ScrubBridgeCreate(b)
		case prot.MsgType(prot.RPCExecuteProcess) | prot.MsgTypeRequest:
			b, err = log.ScrubBridgeExecProcess(b)
		}
		if err != nil {
			brdg.log.WithError(err).Warning("could not scrub bridge payload")
		}
		brdg.log.WithFields(logrus.Fields{
			"payload":    string(b),
			"type":       typ.String(),
			"message-id": id}).Trace("bridge send")
	}

	// Write the message.
	_, err = buf.WriteTo(brdg.conn)
	if err != nil {
		return fmt.Errorf("bridge write: %w", err)
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
	typ := prot.MsgType(call.proc) | prot.MsgTypeRequest
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
