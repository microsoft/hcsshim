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

	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/windowssecuritypolicy"
	"github.com/Microsoft/hcsshim/internal/guest/gcserr"
)

// TODO:
// - Test different error cases
// - Add spans, bridge.Log()
// - b.quitCh is to be used if stop/shutdownContainer validation fails only right?
// - cherry pick commit to add annotations for securityPolicy
// - shimdiag.exe exec uvmID
// TODO: Do we need to support schema1 request types?

// UnknownMessage represents the default handler logic for an unmatched request
// type sent from the bridge.
func UnknownMessage(r *request) error {
	messageType := getMessageType(r.header)
	log.Printf("unknown handler with rpcMessage type %v", msgType(messageType).String())
	return gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: %v", messageType), gcserr.HrNotImpl)
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
	messageType := getMessageType(r.header)
	rpcProcID := rpcProc(msgType(messageType) &^ msgTypeMask)
	if handler, ok = b.rpcHandlerList[rpcProcID]; !ok {
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

	if _, ok := b.rpcHandlerList[rpcProcID]; ok {
		log.Printf("opengcs::bridge - overwriting bridge handler. message-type: %v", rpcProcID.String())
	}

	b.rpcHandlerList[rpcProcID] = handlerFunc
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
	b.HandleFunc(rpcModifySettings, b.modifySettings)
	b.HandleFunc(rpcNegotiateProtocol, b.negotiateProtocol)
	b.HandleFunc(rpcDumpStacks, b.dumpStacks)
	b.HandleFunc(rpcDeleteContainerState, b.deleteContainerState)
	b.HandleFunc(rpcUpdateContainer, b.updateContainer)
	b.HandleFunc(rpcLifecycleNotification, b.lifecycleNotification)
}

type Bridge struct {
	mu sync.Mutex

	// List of handlers for handling different rpc message requests.
	rpcHandlerList map[rpcProc]HandlerFunc
	// Security policy enforcer for c-wcow
	PolicyEnforcer *SecurityPoliyEnforcer

	// hcsshim and inbox GCS connections respectively.
	shimConn     io.ReadWriteCloser
	inboxGCSConn io.ReadWriteCloser

	// Response channels to forward incoming requests to inbox GCS
	// and send responses back to hcsshim respectively.
	sendToGCSChan chan request
	sendToShimCh  chan request

	quitChan chan error
	// brdgErr error
}

type SecurityPoliyEnforcer struct {
	// State required for the security policy enforcement
	policyMutex               sync.Mutex
	securityPolicyEnforcer    windowssecuritypolicy.SecurityPolicyEnforcer
	securityPolicyEnforcerSet bool
	uvmReferenceInfo          string
}

func NewBridge(shimConn io.ReadWriteCloser, inboxGCSConn io.ReadWriteCloser) *Bridge {
	return &Bridge{
		rpcHandlerList: make(map[rpcProc]HandlerFunc),
		shimConn:       shimConn,
		inboxGCSConn:   inboxGCSConn,
		sendToGCSChan:  make(chan request),
		sendToShimCh:   make(chan request),
		quitChan:       make(chan error),
	}
}

func NewPolicyEnforcer(initialEnforcer windowssecuritypolicy.SecurityPolicyEnforcer) *SecurityPoliyEnforcer {
	return &SecurityPoliyEnforcer{
		securityPolicyEnforcerSet: false,
		securityPolicyEnforcer:    initialEnforcer,
	}
}

type messageHeader struct {
	Type msgType
	Size uint32
	ID   int64
}

type bridgeResponse struct {
	// ctx is the context created on request read
	// ctx      context.Context
	header   *messageHeader
	response interface{}
}

type request struct {
	// Context created once received from the bridge.
	// context context.Context
	// header is the wire format message header that preceded the message for
	// this request.
	header [hdrSize]byte
	// TODO: Cleanup. Get the following by unmarshalling when needed?
	// containerID is the id of the container that this message corresponds to.
	// containerID string
	// activityID is the id of the specific activity for this request.
	// activityID string
	// message is the portion of the request that follows the `Header`. This is
	// a json encoded string that MUST contain `prot.MessageBase`.
	message []byte
}

// Helper functions to get message header fields
func getMessageType(header [hdrSize]byte) msgType {
	return msgType(binary.LittleEndian.Uint32(header[hdrOffType:]))
}

func getMessageSize(header [hdrSize]byte) uint32 {
	return binary.LittleEndian.Uint32(header[hdrOffSize:])
}

func getMessageID(header [hdrSize]byte) int64 {
	return int64(binary.LittleEndian.Uint64(header[hdrOffID:]))
}

func readMessage(r io.Reader) (request, error) {
	var h [hdrSize]byte
	_, err := io.ReadFull(r, h[:])
	if err != nil {
		return request{}, err
	}

	//	_, span := oc.StartSpan(context.Background(), "bridge receive read message", oc.WithClientSpanKind)
	//	defer span.End()
	n := getMessageSize(h)
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

	return request{header: h, message: msg}, nil
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
				log.Printf("bridge read from shim connection failed: %s \n", err)
				recverr = errors.Wrap(err, "bridge read from shim connection failed")
				break
			}

			log.Printf("Request from shim: \n Header {Type: %v Size: %v ID: %v }\n msg: %v \n", getMessageType(req.header), getMessageSize(req.header), getMessageID(req.header), string(req.message))
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
					return
				}
			}(req)
		}
	}()
	go func() {
		//var resperr error
		for req := range b.sendToGCSChan {
			// reconstruct message and forward to gcs
			var buf bytes.Buffer
			log.Printf("bridge send to gcs, req %v", req)
			if b.prepareMessageAndSend(&req.header, req.message, &buf, b.inboxGCSConn) != nil {
				// kill bridge?
				log.Printf("err sending message to GCS")
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
			if err := b.prepareMessageAndSend(&req.header, req.message, &buf, b.shimConn); err != nil {
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

func (b *Bridge) prepareMessageAndSend(header *[hdrSize]byte, message []byte, buf *bytes.Buffer, conn io.ReadWriteCloser) error {
	buf.Write(header[:])
	buf.Write(message[:])
	// Write the message.
	_, err := buf.WriteTo(conn)
	if err != nil {
		return fmt.Errorf("bridge write: %s", err)
	}
	return nil
}
