//go:build windows
// +build windows

package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/tracestate"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

type Bridge struct {
	mu        sync.Mutex
	hostState *Host
	// List of handlers for handling different rpc message requests.
	rpcHandlerList map[prot.RPCProc]HandlerFunc

	// hcsshim and inbox GCS connections respectively.
	shimConn     io.ReadWriteCloser
	inboxGCSConn io.ReadWriteCloser

	// Response channels to forward incoming requests to inbox GCS
	// and send responses back to hcsshim respectively.
	sendToGCSCh  chan request
	sendToShimCh chan bridgeResponse
}

// SequenceID is used to correlate requests and responses.
type sequenceID uint64

// messageHeader is the common header present in all communications messages.
type messageHeader struct {
	Type prot.MsgType
	Size uint32
	ID   sequenceID
}

type bridgeResponse struct {
	ctx      context.Context
	header   messageHeader
	response []byte
}

type request struct {
	// Context created once received from the bridge.
	ctx context.Context
	// header is the wire format message header that preceded the message for
	// this request.
	header messageHeader
	// activityID is the id of the specific activity for this request.
	activityID guid.GUID
	// message is the portion of the request that follows the `Header`.
	message []byte
}

func NewBridge(shimConn io.ReadWriteCloser, inboxGCSConn io.ReadWriteCloser, initialEnforcer securitypolicy.SecurityPolicyEnforcer) *Bridge {
	hostState := NewHost(initialEnforcer)
	return &Bridge{
		rpcHandlerList: make(map[prot.RPCProc]HandlerFunc),
		hostState:      hostState,
		shimConn:       shimConn,
		inboxGCSConn:   inboxGCSConn,
		sendToGCSCh:    make(chan request),
		sendToShimCh:   make(chan bridgeResponse),
	}
}

func NewPolicyEnforcer(initialEnforcer securitypolicy.SecurityPolicyEnforcer) *SecurityPolicyEnforcer {
	return &SecurityPolicyEnforcer{
		securityPolicyEnforcerSet: false,
		securityPolicyEnforcer:    initialEnforcer,
	}
}

// UnknownMessage represents the default handler logic for an unmatched request
// type sent from the bridge.
func UnknownMessage(r *request) error {
	log.G(r.ctx).Debugf("bridge: function not supported, header type %v", prot.MsgType(r.header.Type).String())
	return gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: %v", r.header.Type), gcserr.HrNotImpl)
}

// HandlerFunc is an adapter to use functions as handlers.
type HandlerFunc func(*request) error

func (b *Bridge) getRequestHandler(r *request) (HandlerFunc, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var handler HandlerFunc
	var ok bool
	messageType := r.header.Type
	rpcProcID := prot.RPCProc(messageType &^ prot.MsgTypeMask)
	if handler, ok = b.rpcHandlerList[rpcProcID]; !ok {
		return nil, UnknownMessage(r)
	}
	return handler, nil
}

// ServeMsg serves request by calling appropriate handler functions.
func (b *Bridge) ServeMsg(r *request) error {
	if r == nil {
		panic("bridge: nil request to handler")
	}

	var handler HandlerFunc
	var err error
	if handler, err = b.getRequestHandler(r); err != nil {
		return UnknownMessage(r)
	}
	return handler(r)
}

// Handle registers the handler for the given message id and protocol version.
func (b *Bridge) Handle(rpcProcID prot.RPCProc, handlerFunc HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if handlerFunc == nil {
		panic("empty function handler")
	}

	if _, ok := b.rpcHandlerList[rpcProcID]; ok {
		logrus.WithFields(logrus.Fields{
			"message-type": rpcProcID.String(),
		}).Warn("overwriting bridge handler")
	}

	b.rpcHandlerList[rpcProcID] = handlerFunc
}

func (b *Bridge) HandleFunc(rpcProcID prot.RPCProc, handler func(*request) error) {
	if handler == nil {
		panic("bridge: nil handler func")
	}

	b.Handle(rpcProcID, HandlerFunc(handler))
}

// AssignHandlers creates and assigns appropriate event handlers
// for the different bridge message types.
func (b *Bridge) AssignHandlers() {
	b.HandleFunc(prot.RPCCreate, b.createContainer)
	b.HandleFunc(prot.RPCStart, b.startContainer)
	b.HandleFunc(prot.RPCShutdownGraceful, b.shutdownGraceful)
	b.HandleFunc(prot.RPCShutdownForced, b.shutdownForced)
	b.HandleFunc(prot.RPCExecuteProcess, b.executeProcess)
	b.HandleFunc(prot.RPCWaitForProcess, b.waitForProcess)
	b.HandleFunc(prot.RPCSignalProcess, b.signalProcess)
	b.HandleFunc(prot.RPCResizeConsole, b.resizeConsole)
	b.HandleFunc(prot.RPCGetProperties, b.getProperties)
	b.HandleFunc(prot.RPCModifySettings, b.modifySettings)
	b.HandleFunc(prot.RPCNegotiateProtocol, b.negotiateProtocol)
	b.HandleFunc(prot.RPCDumpStacks, b.dumpStacks)
	b.HandleFunc(prot.RPCDeleteContainerState, b.deleteContainerState)
	b.HandleFunc(prot.RPCUpdateContainer, b.updateContainer)
	b.HandleFunc(prot.RPCLifecycleNotification, b.lifecycleNotification)
}

// readMessage reads the message from io.Reader
func readMessage(r io.Reader) (messageHeader, []byte, error) {
	var h [prot.HdrSize]byte
	_, err := io.ReadFull(r, h[:])
	if err != nil {
		return messageHeader{}, nil, err
	}
	var header messageHeader
	buf := bytes.NewReader(h[:])
	err = binary.Read(buf, binary.LittleEndian, &header)
	if err != nil {
		logrus.WithError(err).Errorf("error reading message header")
		return messageHeader{}, nil, err
	}

	n := header.Size
	if n < prot.HdrSize || n > prot.MaxMsgSize {
		logrus.Errorf("invalid message size %d", n)
		return messageHeader{}, nil, fmt.Errorf("invalid message size %d: %w", n, err)
	}

	n -= prot.HdrSize
	msg := make([]byte, n)
	_, err = io.ReadFull(r, msg)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return messageHeader{}, nil, err
	}

	return header, msg, nil
}

func isLocalDisconnectError(err error) bool {
	return errors.Is(err, windows.WSAECONNABORTED)
}

// Sends request to the inbox GCS channel
func (b *Bridge) forwardRequestToGcs(req *request) {
	b.sendToGCSCh <- *req
}

func getContextAndSpan(baseSpanCtx *prot.Ocspancontext) (context.Context, *trace.Span) {
	var ctx context.Context
	var span *trace.Span
	if baseSpanCtx != nil {
		sc := trace.SpanContext{}
		if bytes, err := hex.DecodeString(baseSpanCtx.TraceID); err == nil {
			copy(sc.TraceID[:], bytes)
		}
		if bytes, err := hex.DecodeString(baseSpanCtx.SpanID); err == nil {
			copy(sc.SpanID[:], bytes)
		}
		sc.TraceOptions = trace.TraceOptions(baseSpanCtx.TraceOptions)
		if baseSpanCtx.Tracestate != "" {
			if bytes, err := base64.StdEncoding.DecodeString(baseSpanCtx.Tracestate); err == nil {
				var entries []tracestate.Entry
				if err := json.Unmarshal(bytes, &entries); err == nil {
					if ts, err := tracestate.New(nil, entries...); err == nil {
						sc.Tracestate = ts
					}
				}
			}
		}
		ctx, span = oc.StartSpanWithRemoteParent(
			context.Background(),
			"sidecar::request",
			sc,
			oc.WithServerSpanKind,
		)
	} else {
		ctx, span = oc.StartSpan(
			context.Background(),
			"sidecar::request",
			oc.WithServerSpanKind,
		)
	}

	return ctx, span
}

func sendWithContextCancel[T any](ctx context.Context, sendCh chan<- T, msg T) error {
	select {
	case sendCh <- msg:
		// Sent successfully
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Sends response to the hcsshim channel
func (b *Bridge) sendResponseToShim(ctx context.Context, rpcProcType prot.RPCProc, id sequenceID, response interface{}) error {
	respType := prot.MsgTypeResponse | prot.MsgType(rpcProcType)
	msgb, err := json.Marshal(response)
	if err != nil {
		return err
	}
	msgHeader := messageHeader{
		Type: respType,
		Size: uint32(len(msgb) + prot.HdrSize),
		ID:   id,
	}

	resp := bridgeResponse{
		ctx:      ctx,
		header:   msgHeader,
		response: msgb,
	}

	return sendWithContextCancel(ctx, b.sendToShimCh, resp)
}

// ListenAndServeShimRequests listens to messages on the hcsshim
// and inbox GCS connections and schedules them for processing.
// After processing, messages are forwarded to inbox GCS on success
// and responses from inbox GCS or error messages are sent back
// to hcsshim via bridge connection.
func (b *Bridge) ListenAndServeShimRequests() error {
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	shimRequestChan := make(chan request)
	// goroutines share the error channel to send error responses
	// back to hcsshim. Therefore use buffered error channel.
	sidecarErrChan := make(chan error, 5)

	// Goroutine 1: Listen to requests from hcsshim
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(shimRequestChan)

		var recverr error
		br := bufio.NewReader(b.shimConn)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				header, msg, err := readMessage(br)
				if err != nil {
					if errors.Is(err, io.EOF) || isLocalDisconnectError(err) {
						// hcsshim handles these errors explicitly. Therefore,
						// returning the exact same error.
						recverr = err
					} else {
						recverr = fmt.Errorf("bridge read from shim connection failed: %w", err)
					}
					logrus.Error(recverr)
					_ = sendWithContextCancel(ctx, sidecarErrChan, recverr)
					return
				}

				var msgBase prot.RequestBase
				_ = json.Unmarshal(msg, &msgBase)
				reqCtx, span := getContextAndSpan(msgBase.OpenCensusSpanContext)
				span.AddAttributes(
					trace.Int64Attribute("message-id", int64(header.ID)),
					trace.StringAttribute("message-type", header.Type.String()),
					trace.StringAttribute("activityID", msgBase.ActivityID.String()),
					trace.StringAttribute("containerID", msgBase.ContainerID))

				req := request{
					ctx:        reqCtx,
					activityID: msgBase.ActivityID,
					header:     header,
					message:    msg,
				}
				if err := sendWithContextCancel(ctx, shimRequestChan, req); err != nil {
					log.G(ctx).WithError(err).Error("failed to send request to shimRequestChan")
					return
				}
			}
		}
	}()
	//  Goroutine 2: Process each bridge request received from shim asynchronously.
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			// Requests are served sequentially to avoid
			// racing/reordering of incoming message order.
			// This becomes important for confidential cases
			// where the shim could be compromised and replay
			// messages out of order.
			select {
			case <-ctx.Done():
				return
			case req, ok := <-shimRequestChan:
				if !ok {
					return
				}
				if err := b.ServeMsg(&req); err != nil {
					log.G(req.ctx).WithError(err).Errorf("failed to serve request: %v", req.header.Type.String())
					// In case of error, create appropriate response message to
					// be sent to hcsshim.
					resp := &prot.ResponseBase{
						Result:       int32(windows.ERROR_GEN_FAILURE),
						ErrorMessage: err.Error(),
						ActivityID:   req.activityID,
					}
					setErrorForResponseBase(resp, err, "gcs-sidecar" /* moduleName */)
					// Prepares and sends message to hcsshim via b.sendToShimCh
					responseErr := b.sendResponseToShim(req.ctx, prot.RPCProc(prot.MsgTypeResponse), req.header.ID, resp)
					if responseErr != nil {
						log.G(req.ctx).WithError(err).Errorf("failed to send response to shim")
						_ = sendWithContextCancel(ctx, sidecarErrChan, responseErr)
						return
					}
				}
			}
		}
	}()
	//  Goroutine 3: Forrward requests to inbox GCS
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case req, ok := <-b.sendToGCSCh:
				if !ok {
					return
				}

				// Forward message to gcs
				log.G(req.ctx).Tracef("bridge send to gcs, req %v, %v", req.header.Type.String(), string(req.message))
				buffer, err := b.prepareResponseMessage(req.header, req.message)
				if err != nil {
					responseErr := fmt.Errorf("error preparing response: %w", err)
					_ = sendWithContextCancel(ctx, sidecarErrChan, responseErr)
					return
				}

				_, err = buffer.WriteTo(b.inboxGCSConn)
				if err != nil {
					responseErr := fmt.Errorf("err forwarding shim req to inbox GCS: %w", err)
					_ = sendWithContextCancel(ctx, sidecarErrChan, responseErr)
					return
				}
			}
		}
	}()
	// Goroutine 4: Receive response from gcs and forward to hcsshim
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				header, message, err := readMessage(b.inboxGCSConn)
				if err != nil {
					var recverr error
					if errors.Is(err, io.EOF) || isLocalDisconnectError(err) {
						// hcsshim handles these errors explicitly. Therefore,
						// returning the exact same error.
						recverr = err
					} else {
						recverr = fmt.Errorf("bridge read from gcs failed: %w", err)
					}
					logrus.Error(recverr)
					_ = sendWithContextCancel(ctx, sidecarErrChan, recverr)
					return
				}

				// Forward to shim
				resp := bridgeResponse{
					ctx:      context.Background(),
					header:   header,
					response: message,
				}
				if err := sendWithContextCancel(ctx, b.sendToShimCh, resp); err != nil {
					log.G(ctx).WithError(err).Error("failed to send request to b.sendToShimCh")
					return
				}
			}
		}
	}()
	// Goroutine 5: Send response to hcsshim
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-b.sendToShimCh:
				if !ok {
					return
				}

				// Send response to shim
				logrus.Tracef("Send response to shim. Header:{ID: %v, Type: %v, Size: %v} msg: %v", resp.header.ID,
					resp.header.Type, resp.header.Size, string(resp.response))
				buffer, err := b.prepareResponseMessage(resp.header, resp.response)
				if err != nil {
					responseErr := fmt.Errorf("error preparing response: %w", err)
					_ = sendWithContextCancel(ctx, sidecarErrChan, responseErr)
					return
				}
				_, sendErr := buffer.WriteTo(b.shimConn)
				if sendErr != nil {
					responseErr := fmt.Errorf("err sending response to shim: %w", sendErr)
					_ = sendWithContextCancel(ctx, sidecarErrChan, responseErr)
					return
				}
			}
		}
	}()

	var finalErr error
	select {
	case finalErr = <-sidecarErrChan:
		cancel()
	case <-ctx.Done():
		finalErr = ctx.Err()
		cancel()
	}

	// Close read b.ShimConn and b.inboxGCSConn
	_ = b.shimConn.Close()
	_ = b.inboxGCSConn.Close()

	// Close sidecarErrChan after all the goroutines have finished
	// sending to it.
	wg.Wait()
	close(sidecarErrChan)

	return finalErr
}

// Prepare response message
func (b *Bridge) prepareResponseMessage(header messageHeader, message []byte) (bytes.Buffer, error) {
	// Create a buffer to hold the serialized header data
	var headerBuf bytes.Buffer
	err := binary.Write(&headerBuf, binary.LittleEndian, header)
	if err != nil {
		return headerBuf, err
	}

	// Write message header followed by actual payload.
	var buf bytes.Buffer
	buf.Write(headerBuf.Bytes())
	buf.Write(message[:])
	return buf, nil
}

// setErrorForResponseBase modifies the passed-in ResponseBase to
// contain information pertaining to the given error.
func setErrorForResponseBase(response *prot.ResponseBase, errForResponse error, moduleName string) {
	hresult, errorMessage, newRecord := commonutils.SetErrorForResponseBaseUtil(errForResponse, moduleName)
	response.Result = int32(hresult)
	response.ErrorMessage = errorMessage
	response.ErrorRecords = append(response.ErrorRecords, newRecord)
}
