// Package bridge defines the bridge struct, which implements the control loop
// and functions of the GCS's bridge client.
package bridge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// NotSupported represents the default handler logic for an unmatched
// request type sent from the bridge.
func NotSupported(w ResponseWriter, r *Request) {
	w.Error("", gcserr.WrapHresult(errors.Errorf("bridge: function not supported, header type: 0x%x", r.Header.Type), gcserr.HrNotImpl))
}

// NotSupportedHandler creates a default HandlerFunc out of
// the NotSupported handler logic.
func NotSupportedHandler() Handler {
	return HandlerFunc(NotSupported)
}

// Handler responds to a bridge request.
type Handler interface {
	ServeMsg(ResponseWriter, *Request)
}

// HandlerFunc is an adapter to use functions as handlers.
type HandlerFunc func(ResponseWriter, *Request)

// ServeMsg calls f(w, r).
func (f HandlerFunc) ServeMsg(w ResponseWriter, r *Request) {
	f(w, r)
}

// Mux is a protocol multiplexer for request response pairs
// following the bridge protocol.
type Mux struct {
	mu sync.Mutex
	m  map[prot.MessageIdentifier]Handler
}

// NewBridgeMux creates a default bridge multiplexer.
func NewBridgeMux() *Mux {
	return &Mux{m: make(map[prot.MessageIdentifier]Handler)}
}

// Handle registers the handler for the given message id.
func (mux *Mux) Handle(id prot.MessageIdentifier, handler Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if handler == nil {
		panic("bridge: nil handler")
	}

	if _, ok := mux.m[id]; ok {
		logrus.Infof("bridge: overwriting bridge handler for type: 0x%x", id)
	}

	mux.m[id] = handler
}

// HandleFunc registers the handler function for the given message id.
func (mux *Mux) HandleFunc(id prot.MessageIdentifier, handler func(ResponseWriter, *Request)) {
	if handler == nil {
		panic("bridge: nil handler func")
	}

	mux.Handle(id, HandlerFunc(handler))
}

// Handler returns the handler to use for the given request type.
func (mux *Mux) Handler(r *Request) Handler {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if r == nil {
		panic("bridge: nil request to handler")
	}

	var h Handler
	var ok bool
	if h, ok = mux.m[r.Header.Type]; !ok {
		return NotSupportedHandler()
	}

	return h
}

// ServeMsg dispatches the request to the handler whose
// type matches the request type.
func (mux *Mux) ServeMsg(w ResponseWriter, r *Request) {
	h := mux.Handler(r)
	h.ServeMsg(w, r)
}

// Request is the bridge request that has been sent.
type Request struct {
	Header  *prot.MessageHeader
	Message []byte
}

// ResponseWriter is the dispatcher used to construct the Bridge response.
type ResponseWriter interface {
	// Header is the request header that was requested.
	Header() *prot.MessageHeader
	// Write a successful response message.
	Write(interface{})
	// Error writes the provided error as a response to the message correlated
	// with the activity ID passed. If the activity ID is the empty string it
	// will be translated to an empty guid.
	Error(string, error)
}

type bridgeResponse struct {
	header   *prot.MessageHeader
	response interface{}
}

type requestResponseWriter struct {
	header      *prot.MessageHeader
	respChan    chan bridgeResponse
	respWritten bool
}

func (w *requestResponseWriter) Header() *prot.MessageHeader {
	return w.header
}

func (w *requestResponseWriter) Write(r interface{}) {
	w.respChan <- bridgeResponse{header: w.header, response: r}
	w.respWritten = true
}

func (w *requestResponseWriter) Error(activityID string, err error) {
	if activityID == "" {
		activityID = "00000000-0000-0000-0000-000000000000"
	}

	resp := &prot.MessageResponseBase{ActivityID: activityID}
	setErrorForResponseBase(resp, err)
	w.Write(resp)
}

// Bridge defines the bridge client in the GCS. It acts in many
// ways analogous to go's `http` package and multiplexer.
//
// It has two fundamentally different dispatch options:
//
// 1. Request/Response where using the `Handler` a request
//    of a given type will be dispatched to the apprpriate handler
//    and an appropriate `ResponseWriter` will respond to exactly
//    that request that caused the dispatch.
//
// 2. `PublishNotification` where a notification that was not initiated
//    by a request from any client can be written to the bridge at any time
//    in any order.
type Bridge struct {
	// Transport is the transport interface used by the bridge.
	Transport transport.Transport

	// Handler to invoke when messages are received.
	Handler Handler

	// commandConn is the Connection the bridge receives commands (such as
	// ComputeSystemCreate) over.
	commandConn transport.Connection

	// responseChan is the response channel used for both request/response
	// and publish notification workflows.
	responseChan chan bridgeResponse

	// Core - TODO: Remove this and use the mux!
	coreint core.Core

	// testing hook to close the bridge ListenAndServe() method.
	quitChan chan bool
}

// AssignHandlers creates and assigns the appropriate bridge
// events to be listen for and intercepted on `mux` before forwarding
// to `gcs` for handling.
func (b *Bridge) AssignHandlers(mux *Mux, gcs core.Core) {
	b.coreint = gcs
	mux.HandleFunc(prot.ComputeSystemCreateV1, b.createContainer)
	mux.HandleFunc(prot.ComputeSystemExecuteProcessV1, b.execProcess)
	mux.HandleFunc(prot.ComputeSystemShutdownForcedV1, b.killContainer)
	mux.HandleFunc(prot.ComputeSystemShutdownGracefulV1, b.shutdownContainer)
	mux.HandleFunc(prot.ComputeSystemSignalProcessV1, b.signalProcess)
	mux.HandleFunc(prot.ComputeSystemGetPropertiesV1, b.listProcesses)
	mux.HandleFunc(prot.ComputeSystemWaitForProcessV1, b.waitOnProcess)
	mux.HandleFunc(prot.ComputeSystemResizeConsoleV1, b.resizeConsole)
	mux.HandleFunc(prot.ComputeSystemModifySettingsV1, b.modifySettings)
}

// ListenAndServe connects to the bridge transport, listens for
// messages and dispatches the appropriate handlers to handle each
// event in an asynchronous manner.
func (b *Bridge) ListenAndServe() (conerr error) {
	const commandPort uint32 = 0x40000000

	var err error
	b.commandConn, err = b.Transport.Dial(commandPort)
	if err != nil {
		return errors.Wrap(err, "bridge: failed creating the command Connection")
	}
	logrus.Info("bridge: successfully connected to the HCS via HyperV_Socket")

	requestChan := make(chan *Request)
	requestErrChan := make(chan error)
	b.responseChan = make(chan bridgeResponse)
	responseErrChan := make(chan error)
	b.quitChan = make(chan bool)

	defer close(requestChan)
	defer close(requestErrChan)
	defer close(b.responseChan)
	defer close(responseErrChan)
	defer close(b.quitChan)

	// Receive bridge requests and schedule them to be processed.
	go func() {
		for {
			header := &prot.MessageHeader{}
			if err := binary.Read(b.commandConn, binary.LittleEndian, header); err != nil {
				requestErrChan <- errors.Wrap(err, "bridge: failed reading message header")
				continue
			}
			message := make([]byte, header.Size-prot.MessageHeaderSize)
			if _, err := io.ReadFull(b.commandConn, message); err != nil {
				requestErrChan <- errors.Wrap(err, "bridge: failed reading message payload")
				continue
			}
			logrus.Infof("bridge: read message type: %v", header.Type)
			logrus.Infof("bridge: read message '%s'", message)
			requestChan <- &Request{header, message}
		}
	}()
	// Process each bridge request async and create the response writer.
	go func() {
		for req := range requestChan {
			go func(r *Request) {
				wr := &requestResponseWriter{
					header: &prot.MessageHeader{
						Type: prot.GetResponseIdentifier(r.Header.Type),
						ID:   r.Header.ID,
					},
					respChan: b.responseChan,
				}
				b.Handler.ServeMsg(wr, r)
				if !wr.respWritten {
					logrus.Errorf("bridge: request: ID: 0x%x, Type: %d failed to write a response.", r.Header.ID, r.Header.Type)
				}
			}(req)
		}
	}()
	// Process each bridge response sync. This channel is for request/response and publish workflows.
	go func() {
		for resp := range b.responseChan {
			responseBytes, err := json.Marshal(resp.response)
			if err != nil {
				responseErrChan <- errors.Wrapf(err, "bridge: failed to marshal JSON for response \"%v\"", resp.response)
				continue
			}
			resp.header.Size = uint32(len(responseBytes) + prot.MessageHeaderSize)
			if err := binary.Write(b.commandConn, binary.LittleEndian, resp.header); err != nil {
				responseErrChan <- errors.Wrap(err, "bridge: failed writing message header")
				continue
			}

			if _, err := b.commandConn.Write(responseBytes); err != nil {
				responseErrChan <- errors.Wrap(err, "bridge: failed writing message payload")
				continue
			}
			logrus.Infof("bridge: response sent: '%s' to HCS", responseBytes)
		}
	}()
	// If we get any errors. We return from Listen and shutdown the bridge connection.
	select {
	case conerr = <-requestErrChan:
		break
	case conerr = <-responseErrChan:
		break
	case <-b.quitChan:
		break
	}
	return conerr
}

// PublishNotification writes a specific notification to the bridge.
func (b *Bridge) PublishNotification(n *prot.ContainerNotification) {
	if n == nil {
		panic("bridge: cannot publish nil notification")
	}

	resp := bridgeResponse{
		header: &prot.MessageHeader{
			Type: prot.ComputeSystemNotificationV1,
			ID:   0,
		},
		response: n,
	}
	b.responseChan <- resp
}

func (b *Bridge) createContainer(w ResponseWriter, r *Request) {
	var request prot.ContainerCreate
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", r.Message))
		return
	}

	// The request contains a JSON string field which is equivalent to a
	// CreateContainerInfo struct.
	var settings prot.VMHostedContainerSettings
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.ContainerConfig), &settings); err != nil {
		w.Error(request.ActivityID, errors.Wrapf(err, "failed to unmarshal JSON for ContainerConfig \"%s\"", request.ContainerConfig))
		return
	}

	id := request.ContainerID
	if err := b.coreint.CreateContainer(id, settings); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.ContainerCreateResponse{
		MessageResponseBase: &prot.MessageResponseBase{
			ActivityID: request.ActivityID,
		},
		SelectedProtocolVersion: prot.PvV3,
	}

	exitCodeFn, err := b.coreint.WaitContainer(id)
	if err != nil {
		logrus.Error(err)
	}

	go func() {
		exitCode := exitCodeFn()
		notification := &prot.ContainerNotification{
			MessageBase: &prot.MessageBase{
				ContainerID: id,
				ActivityID:  request.ActivityID,
			},
			Type:       prot.NtUnexpectedExit, // TODO: Support different exit types.
			Operation:  prot.AoNone,
			Result:     int32(exitCode),
			ResultInfo: "",
		}
		b.PublishNotification(notification)
	}()

	w.Write(response)
}

func (b *Bridge) execProcess(w ResponseWriter, r *Request) {
	var request prot.ContainerExecuteProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	// The request contains a JSON string field which is equivalent to an
	// ExecuteProcessInfo struct.
	var params prot.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.Settings.ProcessParameters), &params); err != nil {
		w.Error(request.ActivityID, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters))
		return
	}

	stdioSet, err := connectStdio(b.Transport, params, request.Settings.VsockStdioRelaySettings)
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}
	var pid int
	if params.IsExternal {
		pid, err = b.coreint.RunExternalProcess(params, stdioSet)
	} else {
		pid, err = b.coreint.ExecProcess(request.ContainerID, params, stdioSet)
	}

	if err != nil {
		stdioSet.Close() // stdioSet will be eventually closed by coreint on success
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.ContainerExecuteProcessResponse{
		MessageResponseBase: &prot.MessageResponseBase{
			ActivityID: request.ActivityID,
		},
		ProcessID: uint32(pid),
	}
	w.Write(response)
}

func (b *Bridge) killContainer(w ResponseWriter, r *Request) {
	b.signalContainer(w, r, oslayer.SIGKILL)
}

func (b *Bridge) shutdownContainer(w ResponseWriter, r *Request) {
	b.signalContainer(w, r, oslayer.SIGTERM)
}

// signalContainer is not a handler func. This is because the actual signal is
// implied based on the message type.
func (b *Bridge) signalContainer(w ResponseWriter, r *Request, signal oslayer.Signal) {
	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	if err := b.coreint.SignalContainer(request.ContainerID, signal); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

func (b *Bridge) signalProcess(w ResponseWriter, r *Request) {
	var request prot.ContainerSignalProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	if err := b.coreint.SignalProcess(int(request.ProcessID), request.Options); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

func (b *Bridge) listProcesses(w ResponseWriter, r *Request) {
	var request prot.ContainerGetProperties
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}
	id := request.ContainerID

	processes, err := b.coreint.ListProcesses(id)
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	processJSON, err := json.Marshal(processes)
	if err != nil {
		w.Error(request.ActivityID, errors.Wrapf(err, "failed to marshal processes into JSON: %v", processes))
		return
	}

	response := &prot.ContainerGetPropertiesResponse{
		MessageResponseBase: &prot.MessageResponseBase{
			ActivityID: request.ActivityID,
		},
		Properties: string(processJSON),
	}
	w.Write(response)
}

func (b *Bridge) waitOnProcess(w ResponseWriter, r *Request) {
	var request prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	exitCodeChan, doneChan, err := b.coreint.WaitProcess(int(request.ProcessID))
	if err != nil {
		w.Error(request.ActivityID, err)
		return
	}
	defer close(doneChan)

	select {
	case exitCode := <-exitCodeChan:
		response := &prot.ContainerWaitForProcessResponse{
			MessageResponseBase: &prot.MessageResponseBase{
				ActivityID: request.ActivityID,
			},
			ExitCode: uint32(exitCode),
		}
		w.Write(response)
	case <-time.After(time.Duration(request.TimeoutInMs) * time.Millisecond):
		w.Error(request.ActivityID, gcserr.NewHresultError(gcserr.HvVmcomputeTimeout))
	}

	// If we timed out or if we got the exit code. Acknowledge we no longer want to wait.
	doneChan <- true
}

func (b *Bridge) resizeConsole(w ResponseWriter, r *Request) {
	var request prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(r.Message, &request); err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	if err := b.coreint.ResizeConsole(int(request.ProcessID), request.Height, request.Width); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

func (b *Bridge) modifySettings(w ResponseWriter, r *Request) {
	request, err := prot.UnmarshalContainerModifySettings(r.Message)
	if err != nil {
		w.Error("", errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", r.Message))
		return
	}

	if err := b.coreint.ModifySettings(request.ContainerID, request.Request); err != nil {
		w.Error(request.ActivityID, err)
		return
	}

	response := &prot.MessageResponseBase{
		ActivityID: request.ActivityID,
	}
	w.Write(response)
}

// setErrorForResponseBase modifies the passed-in MessageResponseBase to
// contain information pertaining to the given error.
func setErrorForResponseBase(response *prot.MessageResponseBase, errForResponse error) {
	errorMessage := errForResponse.Error()
	stackString := ""
	fileName := ""
	lineNumber := -1
	functionName := ""
	if stack := gcserr.BaseStackTrace(errForResponse); stack != nil {
		bottomFrame := stack[0]
		stackString = fmt.Sprintf("%+v", stack)
		fileName = fmt.Sprintf("%s", bottomFrame)
		lineNumberStr := fmt.Sprintf("%d", bottomFrame)
		var err error
		lineNumber, err = strconv.Atoi(lineNumberStr)
		if err != nil {
			logrus.Error(errors.Wrapf(err, "failed to parse \"%s\" as line number of error, using -1 instead", lineNumberStr))
			lineNumber = -1
		}
		functionName = fmt.Sprintf("%n", bottomFrame)
	}
	hresult, err := gcserr.GetHresult(errForResponse)
	if err != nil {
		// Default to using the generic failure HRESULT.
		hresult = gcserr.HrFail
	}
	response.Result = int32(hresult)
	newRecord := prot.ErrorRecord{
		Result:       int32(hresult),
		Message:      errorMessage,
		StackTrace:   stackString,
		ModuleName:   "gcs",
		FileName:     fileName,
		Line:         uint32(lineNumber),
		FunctionName: functionName,
	}
	response.ErrorRecords = append(response.ErrorRecords, newRecord)
}
