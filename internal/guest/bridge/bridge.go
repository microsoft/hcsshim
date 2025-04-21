//go:build linux
// +build linux

package bridge

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
	"go.opencensus.io/trace/tracestate"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime/hcsv2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
)

// UnknownMessage represents the default handler logic for an unmatched request
// type sent from the bridge.
func UnknownMessage(r *Request) (RequestResponse, error) {
	return nil, gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: %v", r.Header.Type), gcserr.HrNotImpl)
}

// UnknownMessageHandler creates a default HandlerFunc out of the
// UnknownMessage handler logic.
func UnknownMessageHandler() Handler {
	return HandlerFunc(UnknownMessage)
}

// Handler responds to a bridge request.
type Handler interface {
	ServeMsg(*Request) (RequestResponse, error)
}

// HandlerFunc is an adapter to use functions as handlers.
type HandlerFunc func(*Request) (RequestResponse, error)

// ServeMsg calls f(w, r).
func (f HandlerFunc) ServeMsg(r *Request) (RequestResponse, error) {
	return f(r)
}

// Mux is a protocol multiplexer for request response pairs
// following the bridge protocol.
type Mux struct {
	mu sync.Mutex
	m  map[prot.MessageIdentifier]map[prot.ProtocolVersion]Handler
}

// NewBridgeMux creates a default bridge multiplexer.
func NewBridgeMux() *Mux {
	return &Mux{m: make(map[prot.MessageIdentifier]map[prot.ProtocolVersion]Handler)}
}

// Handle registers the handler for the given message id and protocol version.
func (mux *Mux) Handle(id prot.MessageIdentifier, ver prot.ProtocolVersion, handler Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if handler == nil {
		panic("bridge: nil handler")
	}

	if _, ok := mux.m[id]; !ok {
		mux.m[id] = make(map[prot.ProtocolVersion]Handler)
	}

	if _, ok := mux.m[id][ver]; ok {
		logrus.WithFields(logrus.Fields{
			"message-type":     id.String(),
			"protocol-version": ver,
		}).Warn("opengcs::bridge - overwriting bridge handler")
	}

	mux.m[id][ver] = handler
}

// HandleFunc registers the handler function for the given message id and protocol version.
func (mux *Mux) HandleFunc(id prot.MessageIdentifier, ver prot.ProtocolVersion, handler func(*Request) (RequestResponse, error)) {
	if handler == nil {
		panic("bridge: nil handler func")
	}

	mux.Handle(id, ver, HandlerFunc(handler))
}

// Handler returns the handler to use for the given request type.
func (mux *Mux) Handler(r *Request) Handler {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if r == nil {
		panic("bridge: nil request to handler")
	}

	var m map[prot.ProtocolVersion]Handler
	var ok bool
	if m, ok = mux.m[r.Header.Type]; !ok {
		return UnknownMessageHandler()
	}

	var h Handler
	if h, ok = m[r.Version]; !ok {
		return UnknownMessageHandler()
	}

	return h
}

// ServeMsg dispatches the request to the handler whose
// type matches the request type.
func (mux *Mux) ServeMsg(r *Request) (RequestResponse, error) {
	h := mux.Handler(r)
	return h.ServeMsg(r)
}

// Request is the bridge request that has been sent.
type Request struct {
	// Context is the request context received from the bridge.
	Context context.Context
	// Header is the wire format message header that preceded the message for
	// this request.
	Header *prot.MessageHeader
	// ContainerID is the id of the container that this message corresponds to.
	ContainerID string
	// ActivityID is the id of the specific activity for this request.
	ActivityID string
	// Message is the portion of the request that follows the `Header`. This is
	// a json encoded string that MUST contain `prot.MessageBase`.
	Message []byte
	// Version is the version of the protocol that `Header` and `Message` were
	// sent in.
	Version prot.ProtocolVersion
}

// RequestResponse is the base response for any bridge message request.
type RequestResponse interface {
	Base() *prot.MessageResponseBase
}

type bridgeResponse struct {
	// ctx is the context created on request read
	ctx      context.Context
	header   *prot.MessageHeader
	response interface{}
}

// Bridge defines the bridge client in the GCS. It acts in many ways analogous
// to go's `http` package and multiplexer.
//
// It has two fundamentally different dispatch options:
//
//  1. Request/Response where using the `Handler` a request
//     of a given type will be dispatched to the appropriate handler
//     and an appropriate response will respond to exactly that request that
//     caused the dispatch.
//
//  2. `PublishNotification` where a notification that was not initiated
//     by a request from any client can be written to the bridge at any time
//     in any order.
type Bridge struct {
	// Handler to invoke when messages are received.
	Handler Handler
	// EnableV4 enables the v4+ bridge and the schema v2+ interfaces.
	EnableV4 bool

	// responseChan is the response channel used for both request/response
	// and publish notification workflows.
	responseChan chan bridgeResponse

	hostState *hcsv2.Host

	quitChan chan bool
	// hasQuitPending indicates the bridge is shutting down and cause no more requests to be Read.
	hasQuitPending atomic.Bool

	protVer prot.ProtocolVersion
}

// AssignHandlers creates and assigns the appropriate bridge
// events to be listen for and intercepted on `mux` before forwarding
// to `gcs` for handling.
func (b *Bridge) AssignHandlers(mux *Mux, host *hcsv2.Host) {
	b.hostState = host

	// These are PvInvalid because they will be called previous to any protocol
	// negotiation so they respond only when the protocols are not known.
	if b.EnableV4 {
		mux.HandleFunc(prot.ComputeSystemNegotiateProtocolV1, prot.PvInvalid, b.negotiateProtocolV2)
	}

	if b.EnableV4 {
		// v4 specific handlers
		mux.HandleFunc(prot.ComputeSystemStartV1, prot.PvV4, b.startContainerV2)
		mux.HandleFunc(prot.ComputeSystemCreateV1, prot.PvV4, b.createContainerV2)
		mux.HandleFunc(prot.ComputeSystemExecuteProcessV1, prot.PvV4, b.execProcessV2)
		mux.HandleFunc(prot.ComputeSystemShutdownForcedV1, prot.PvV4, b.killContainerV2)
		mux.HandleFunc(prot.ComputeSystemShutdownGracefulV1, prot.PvV4, b.shutdownContainerV2)
		mux.HandleFunc(prot.ComputeSystemSignalProcessV1, prot.PvV4, b.signalProcessV2)
		mux.HandleFunc(prot.ComputeSystemGetPropertiesV1, prot.PvV4, b.getPropertiesV2)
		mux.HandleFunc(prot.ComputeSystemWaitForProcessV1, prot.PvV4, b.waitOnProcessV2)
		mux.HandleFunc(prot.ComputeSystemResizeConsoleV1, prot.PvV4, b.resizeConsoleV2)
		mux.HandleFunc(prot.ComputeSystemModifySettingsV1, prot.PvV4, b.modifySettingsV2)
		mux.HandleFunc(prot.ComputeSystemDumpStacksV1, prot.PvV4, b.dumpStacksV2)
		mux.HandleFunc(prot.ComputeSystemDeleteContainerStateV1, prot.PvV4, b.deleteContainerStateV2)
	}
}

// ListenAndServe connects to the bridge transport, listens for
// messages and dispatches the appropriate handlers to handle each
// event in an asynchronous manner.
func (b *Bridge) ListenAndServe(bridgeIn io.ReadCloser, bridgeOut io.WriteCloser) error {
	requestChan := make(chan *Request)
	requestErrChan := make(chan error)
	b.responseChan = make(chan bridgeResponse)
	responseErrChan := make(chan error)
	b.quitChan = make(chan bool)

	defer close(b.quitChan)
	defer bridgeOut.Close()
	defer close(responseErrChan)
	defer close(b.responseChan)
	defer close(requestChan)
	defer close(requestErrChan)
	defer bridgeIn.Close()

	// Receive bridge requests and schedule them to be processed.
	go func() {
		var recverr error
		for {
			if !b.hasQuitPending.Load() {
				header := &prot.MessageHeader{}
				if err := binary.Read(bridgeIn, binary.LittleEndian, header); err != nil {
					if err == io.ErrUnexpectedEOF || err == os.ErrClosed { //nolint:errorlint
						break
					}
					recverr = errors.Wrap(err, "bridge: failed reading message header")
					break
				}
				message := make([]byte, header.Size-prot.MessageHeaderSize)
				if _, err := io.ReadFull(bridgeIn, message); err != nil {
					if err == io.ErrUnexpectedEOF || err == os.ErrClosed { //nolint:errorlint
						break
					}
					recverr = errors.Wrap(err, "bridge: failed reading message payload")
					break
				}

				base := prot.MessageBase{}
				// TODO: JTERRY75 - This should fail the request but right
				// now we still forward to the method and let them return
				// this error. Unify the JSON part previous to invoking a
				// request.
				_ = json.Unmarshal(message, &base)

				var ctx context.Context
				var span *trace.Span
				if base.OpenCensusSpanContext != nil {
					sc := trace.SpanContext{}
					if bytes, err := hex.DecodeString(base.OpenCensusSpanContext.TraceID); err == nil {
						copy(sc.TraceID[:], bytes)
					}
					if bytes, err := hex.DecodeString(base.OpenCensusSpanContext.SpanID); err == nil {
						copy(sc.SpanID[:], bytes)
					}
					sc.TraceOptions = trace.TraceOptions(base.OpenCensusSpanContext.TraceOptions)
					if base.OpenCensusSpanContext.Tracestate != "" {
						if bytes, err := base64.StdEncoding.DecodeString(base.OpenCensusSpanContext.Tracestate); err == nil {
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
						"opengcs::bridge::request",
						sc,
						oc.WithServerSpanKind,
					)
				} else {
					ctx, span = oc.StartSpan(
						context.Background(),
						"opengcs::bridge::request",
						oc.WithServerSpanKind,
					)
				}

				span.AddAttributes(
					trace.Int64Attribute("message-id", int64(header.ID)),
					trace.StringAttribute("message-type", header.Type.String()),
					trace.StringAttribute("activityID", base.ActivityID),
					trace.StringAttribute("cid", base.ContainerID))

				entry := log.G(ctx)
				if entry.Logger.GetLevel() > logrus.DebugLevel {
					var err error
					var msgBytes []byte
					switch header.Type {
					case prot.ComputeSystemCreateV1:
						msgBytes, err = log.ScrubBridgeCreate(message)
					case prot.ComputeSystemExecuteProcessV1:
						msgBytes, err = log.ScrubBridgeExecProcess(message)
					default:
						msgBytes = message
					}
					s := string(msgBytes)
					if err != nil {
						entry.WithError(err).Warning("could not scrub bridge payload")
					}
					entry.WithField("message", s).Trace("request read message")
				}
				requestChan <- &Request{
					Context:     ctx,
					Header:      header,
					ContainerID: base.ContainerID,
					ActivityID:  base.ActivityID,
					Message:     message,
					Version:     b.protVer,
				}
			}
		}
		requestErrChan <- recverr
	}()
	// Process each bridge request async and create the response writer.
	go func() {
		for req := range requestChan {
			go func(r *Request) {
				br := bridgeResponse{
					ctx: r.Context,
					header: &prot.MessageHeader{
						Type: prot.GetResponseIdentifier(r.Header.Type),
						ID:   r.Header.ID,
					},
				}
				resp, err := b.Handler.ServeMsg(r)
				if resp == nil {
					resp = &prot.MessageResponseBase{}
				}
				resp.Base().ActivityID = r.ActivityID
				if err != nil {
					span := trace.FromContext(r.Context)
					if span != nil {
						oc.SetSpanStatus(span, err)
					}
					setErrorForResponseBase(resp.Base(), err, "gcs" /* moduleName */)
				}
				br.response = resp
				b.responseChan <- br
			}(req)
		}
	}()
	// Process each bridge response sync. This channel is for request/response and publish workflows.
	go func() {
		var resperr error
		for resp := range b.responseChan {
			responseBytes, err := json.Marshal(resp.response)
			if err != nil {
				resperr = errors.Wrapf(err, "bridge: failed to marshal JSON for response \"%v\"", resp.response)
				break
			}
			resp.header.Size = uint32(len(responseBytes) + prot.MessageHeaderSize)
			if err := binary.Write(bridgeOut, binary.LittleEndian, resp.header); err != nil {
				resperr = errors.Wrap(err, "bridge: failed writing message header")
				break
			}

			if _, err := bridgeOut.Write(responseBytes); err != nil {
				resperr = errors.Wrap(err, "bridge: failed writing message payload")
				break
			}

			s := trace.FromContext(resp.ctx)
			if s != nil {
				log.G(resp.ctx).WithField("message", string(responseBytes)).Trace("request write response")
				s.AddAttributes(trace.StringAttribute("response-message-type", resp.header.Type.String()))
				s.End()
			}
		}
		responseErrChan <- resperr
	}()

	select {
	case err := <-requestErrChan:
		return err
	case err := <-responseErrChan:
		return err
	case <-b.quitChan:
		// The request loop needs to exit so that the teardown process begins.
		// Set the request loop to stop processing new messages
		b.hasQuitPending.Store(true)
		// Wait for the request loop to process its last message. Its possible
		// that if it lost the race with the hasQuitPending it could be stuck in
		// a pending read from bridgeIn. Wait 2 seconds and kill the connection.
		var err error
		select {
		case err = <-requestErrChan:
		case <-time.After(time.Second * 5):
			// Timeout expired first. Close the connection to unblock the read
			if cerr := bridgeIn.Close(); cerr != nil {
				err = errors.Wrap(cerr, "bridge: failed to close bridgeIn")
			}
			<-requestErrChan
		}
		<-responseErrChan
		return err
	}
}

// PublishNotification writes a specific notification to the bridge.
func (b *Bridge) PublishNotification(n *prot.ContainerNotification) {
	ctx, span := oc.StartSpan(context.Background(),
		"opengcs::bridge::PublishNotification",
		oc.WithClientSpanKind)
	span.AddAttributes(trace.StringAttribute("notification", fmt.Sprintf("%+v", n)))
	// DONT defer span.End() here. Publish is odd because bridgeResponse calls
	// `End` on the `ctx` after the response is sent.

	resp := bridgeResponse{
		ctx: ctx,
		header: &prot.MessageHeader{
			Type: prot.ComputeSystemNotificationV1,
			ID:   0,
		},
		response: n,
	}
	b.responseChan <- resp
}

// setErrorForResponseBase modifies the passed-in MessageResponseBase to
// contain information pertaining to the given error.
func setErrorForResponseBase(response *prot.MessageResponseBase, errForResponse error, moduleName string) {
	hresult, errorMessage, newRecord := commonutils.SetErrorForResponseBaseUtil(errForResponse, moduleName)
	response.Result = int32(hresult)
	response.ErrorMessage = errorMessage
	response.ErrorRecords = append(response.ErrorRecords, newRecord)
}
