//go:build windows
// +build windows

package bridge

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/guest/gcserr"
)

// TODO:
// - Test different error cases
// - Add spans, bridge.Log()
// - b.quitCh is to be used if stop/shutdownContainer validation fails only right?
// - cherry pick commit to add annotations for securityPolicy
// - shimdiag.exe exec uvmID
// TODO: Do we need to support schema1 request types?
type requestMessage interface {
	Base() *requestBase
}

type responseMessage interface {
	Base() *responseBase
}

/*
// rpc represents an outstanding rpc request to the guest
type rpc struct {
	proc    rpcProc
	id      int64
	req     requestMessage
	resp    responseMessage
	brdgErr error // error encountered when sending the request or unmarshaling the result
	ch      chan struct{}
}
*/
// TODO: 'B'ridge to 'b'ridge
type Bridge struct {
	shimConn     io.ReadWriteCloser
	inboxGCSConn io.ReadWriteCloser

	mu sync.Mutex
	// TODO (confirm): Security policy enforcer is just a library so we do not
	// need to record messages sent to it.

	// List of handlers for the different type of incoming message requests
	handlerList map[rpcProc]HandlerFunc

	// brdgErr error

	// responseChan is the response channel used for both request/response
	// and publish notification workflows.
	// responseChan chan *rpc

	sendToGCSChan chan request

	sendToShimCh chan request

	// waitCh chan struct{}

	quitChan chan error
}

// TODO: rename request to bridgeMessage
type request struct {
	// Context is the request context received from the bridge.
	// Context context.Context
	// Header is the wire format message header that preceded the message for
	// this request.
	header [hdrSize]byte

	// TODO CLEANUP: Maintaining typ and id separately temporarily (debugging).
	// They can removed for final iteration.
	// msgType = rpcProcID | msgTypeRequest. In string like Request(rpcCreate)
	typ msgType
	id  int64

	// TODO: Populate the following so error messages are sent back to
	// outstanding requests before bridge is killed for error handling!

	// ContainerID is the id of the container that this message corresponds to.
	containerID string
	// ActivityID is the id of the specific activity for this request.
	activityID string

	// Message is the portion of the request that follows the `Header`. This is
	// a json encoded string that MUST contain `prot.MessageBase`.
	message []byte
}

func NewBridge(shimConn io.ReadWriteCloser, inboxGCSConn io.ReadWriteCloser) *Bridge {
	return &Bridge{
		shimConn:      shimConn,
		inboxGCSConn:  inboxGCSConn,
		handlerList:   make(map[rpcProc]HandlerFunc),
		sendToGCSChan: make(chan request),
		sendToShimCh:  make(chan request),
		quitChan:      make(chan error),
	}
}

// UnknownMessage represents the default handler logic for an unmatched request
// type sent from the bridge.
func UnknownMessage(r *request) error {
	rpcType := msgType(r.typ) | msgTypeRequest
	log.Printf("unknown handler of typ %v, rpcProc(rpcType) %v", r.typ.String(), rpcProc(rpcType))
	return gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: %v", r.typ), gcserr.HrNotImpl)
}

// HandlerFunc is an adapter to use functions as handlers.
type HandlerFunc func(*request) error

// ServeMsg serves request by calling appropriate handler functions.
func (b *Bridge) ServeMsg(r *request) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if r == nil {
		panic("bridge: nil request to handler")
	}

	var handler HandlerFunc
	var ok bool
	rpcProcID := rpcProc(r.typ &^ msgTypeMask)
	if handler, ok = b.handlerList[rpcProcID]; !ok {
		return UnknownMessage(r)
	}

	return handler(r)
}

// Handle registers the handler for the given message id and protocol version.
func (b *Bridge) Handle(rpcProcID rpcProc, handlerFunc HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if handlerFunc == nil {
		panic("empty function handler")
	}

	if _, ok := b.handlerList[rpcProcID]; ok {
		log.Printf("opengcs::bridge - overwriting bridge handler. message-type: %v", rpcProcID.String())
	}

	b.handlerList[rpcProcID] = handlerFunc
}

func (b *Bridge) HandleFunc(rpcProcID rpcProc, handler func(*request) error) {
	if handler == nil {
		panic("bridge: nil handler func")
	}

	b.Handle(rpcProcID, HandlerFunc(handler))
}

// AssignHandlers creates and assigns the appropriate bridge
// events to be listen for and intercepted before forwarding
// to inbox gcs for handling.
func (b *Bridge) AssignHandlers() {
	b.HandleFunc(rpcCreate, b.createContainer)
	b.HandleFunc(rpcStart, b.startContainer)
	b.HandleFunc(rpcShutdownGraceful, b.shutdownGraceful)
	b.HandleFunc(rpcShutdownForced, b.shutdownForced)
	b.HandleFunc(rpcExecuteProcess, b.executeProcess)
	b.HandleFunc(rpcWaitForProcess, b.waitForProcess)
	b.HandleFunc(rpcSignalProcess, b.signalProcess)
	b.HandleFunc(rpcResizeConsole, b.resizeConsole)
	b.HandleFunc(rpcGetProperties, b.getProperties)
	b.HandleFunc(rpcModifySettings, b.modifySettings) // TODO: Further dereference request types..To be validated like mounting container layers, data volumes etc
	b.HandleFunc(rpcNegotiateProtocol, b.negotiateProtocol)
	b.HandleFunc(rpcDumpStacks, b.dumpStacks)
	b.HandleFunc(rpcDeleteContainerState, b.deleteContainerState)
	b.HandleFunc(rpcUpdateContainer, b.updateContainer)
	b.HandleFunc(rpcLifecycleNotification, b.lifecycleNotification) // TODO: Validate this request as well?
}

type messageHeader struct {
	Type uint32
	Size uint32
	ID   int64
}

func readMessage(r io.Reader) (request, error) {
	var h [hdrSize]byte
	_, err := io.ReadFull(r, h[:])
	if err != nil {
		return request{}, err
	}
	//	_, span := oc.StartSpan(context.Background(), "bridge receive read message", oc.WithClientSpanKind)
	//	defer span.End()

	typ := msgType(binary.LittleEndian.Uint32(h[hdrOffType:]))
	n := binary.LittleEndian.Uint32(h[hdrOffSize:])
	id := int64(binary.LittleEndian.Uint64(h[hdrOffID:]))

	if n < hdrSize || n > maxMsgSize {
		log.Printf("invalid message size %d", n)
		return request{}, fmt.Errorf("invalid message size %d", n)
	}

	n -= hdrSize
	msg := make([]byte, n)
	_, err = io.ReadFull(r, msg)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return request{}, err
	}
	return request{header: h, typ: typ, id: id, message: msg}, nil
}

func isLocalDisconnectError(err error) bool {
	return errors.Is(err, windows.WSAECONNABORTED)
}

func (b *Bridge) ListenAndServeShimRequests() error {
	shimRequestChan := make(chan request)
	shimRequestErrChan := make(chan error)
	gcsRequestErrChan := make(chan error)

	defer close(gcsRequestErrChan)
	defer b.inboxGCSConn.Close()
	defer close(shimRequestChan)
	defer close(shimRequestErrChan)
	defer b.shimConn.Close()
	defer close(b.sendToShimCh)
	defer close(b.sendToGCSChan)

	// Listen to requests from hcsshim
	go func() {
		var recverr error
		br := bufio.NewReader(b.shimConn)
		for {
			req, err := readMessage(br)
			if err != nil {
				if err == io.EOF || isLocalDisconnectError(err) {
					return
				}
				log.Printf("bridge read from shim failed: %s \n", err)
				recverr = errors.Wrap(err, "bridge read from shim failed:")
				break
			}
			var header messageHeader
			messageTyp := msgType(binary.LittleEndian.Uint32(req.header[hdrOffType:]))
			header.Type = binary.LittleEndian.Uint32(req.header[hdrOffType:])
			header.Size = binary.LittleEndian.Uint32(req.header[hdrOffSize:])
			header.ID = int64(binary.LittleEndian.Uint64(req.header[hdrOffID:]))
			log.Printf("bridge recv from shim: \n Header {Type: %v Size: %v ID: %v }\n msg: %v \n", messageTyp, header.Size, header.ID, string(req.message))
			shimRequestChan <- req
		}
		shimRequestErrChan <- recverr
	}()
	// Process each bridge request async and create the response writer.
	go func() {
		for req := range shimRequestChan {
			go func(req request) {
				// Check if operation is allowed or not for c-wcow
				if err := b.ServeMsg(&req); err != nil {
					// TODO: brdg.log()
					// TODO:
					// 1. Behavior if an operation is not allowed?
					// 2. Code cleanup on error
					// ? b.close(err)
					// b.quitCh <- true // give few seconds delay and close connections?
					b.close(err)
					return
				}

				// If we are here, means that the requested operation is allowed.
				// Forward message to GCS. We handle responses from GCS separately.
				log.Printf("hcsshim receive message redirect")
				b.sendToGCSChan <- req
			}(req)
		}
	}()
	go func() {
		//var resperr error
		for req := range b.sendToGCSChan {
			// reconstruct message and forward to gcs
			var buf bytes.Buffer
			log.Printf("bridge send to gcs")
			if b.prepareMessageAndSend(req.header, req.message, &buf, b.inboxGCSConn) != nil {
				// kill bridge?
				log.Printf("err sending message to ")
			}
		}
	}()

	// Receive response from gcs and forward to hcsshim. If there is
	// a processing error, kill the bridge
	go func() {
		var recverr error
		for {
			req, err := readMessage(b.inboxGCSConn)
			if err != nil {
				if err == io.EOF || isLocalDisconnectError(err) {
					return
				}
				recverr = fmt.Errorf("bridge read from gcs failed: %s", err)
				log.Printf("bridge read from gcs failed: %v", err)
				break
			}

			// Reconstruct message and forward to shim
			b.sendToShimCh <- req
		}
		gcsRequestErrChan <- recverr
	}()

	go func() {
		for req := range b.sendToShimCh {
			// reconstruct message and forward to shim
			var buf bytes.Buffer
			log.Printf("send to shim, req: \n Header: %v \n msg: %v \n", "", string(req.message))
			if err := b.prepareMessageAndSend(req.header, req.message, &buf, b.shimConn); err != nil {
				// kill bridge?
				log.Printf("err sending message to ")
				b.close(err)
			}
		}
	}()

	select {
	case err := <-shimRequestErrChan:
		b.close(err)
		return err
	case err := <-gcsRequestErrChan:
		b.close(err)
		return err

		// TODO: A quit channel that can be used from handler function
		// to indicate an error and shutdown the bridge itself. For example,
		// when requested operation like mount container layers, data mounts
		// are not allowed?
		// case err := <-b.quitCh:
		//	return err

	}
}

func (b *Bridge) close(err error) {
	// TODO: Fail outstanding rpc requests before closing bridge and other channels
	// This is important to do as valid errors need to be recorded by callers and fail
	// the requests.
	/********
	b.mu.Lock()
	for _, req := range b.rpcs {
		b.mu.Unlock()
		var buf bytes.Buffer
		// TODO: send error responses to all outstanding requests before killing bridge?
		resp := responseBase{
			Result:       1, // TODO: Set appropriate result string HRESULT; 0 means no error
			ErrorMessage: fmt.Errorf("%v", err),
			ActivityID:   req.header.ActivityID, // TODO: fill in correct activityID
			ErrorRecords: nil,                   // TODO:  []errorRecord. Has message, stacktrace, file:line etc
		}
		// resp []bytes? or prepareMessageAndSend should take an interface.
		// have a base of header, typ, id, ctrdID, activityID and vary by the message?
		b.prepareMessageAndSend(*req, &buf, b.shimConn)

	}
	*/

	b.shimConn.Close()
	b.inboxGCSConn.Close()
	close(b.quitChan)
}

func (b *Bridge) prepareMessageAndSend(header [hdrSize]byte, message []byte, buf *bytes.Buffer, conn io.ReadWriteCloser) error {
	buf.Write(header[:])
	buf.Write(message[:])
	// Write the message.
	_, err := buf.WriteTo(conn)
	if err != nil {
		return fmt.Errorf("bridge write: %s", err)
	}
	return nil
}
