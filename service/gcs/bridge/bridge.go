// Package bridge defines the bridge struct, which implements the control loop
// and functions of the GCS's bridge client.
package bridge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
)

// bridge defines the bridge client in the GCS.
type bridge struct {
	// tport is the Transport interface used by the bridge.
	tport transport.Transport

	// coreint is the Core interface used by the bridge.
	coreint core.Core

	// printErrors is true if the bridge should print errors which occur.
	printErrors bool

	// commandConn is the Connection the bridge receives commands (such as
	// ComputeSystemCreate) over.
	commandConn transport.Connection

	// writeLock must be held while writing to the transport to ensure that
	// message data is kept together when writing from multiple go routines
	// simultaneously.
	writeLock sync.Mutex
}

// NewBridge produces a new bridge struct using the given Transport and Core
// interfaces.
func NewBridge(tport transport.Transport, coreint core.Core, printErrors bool) *bridge {
	return &bridge{
		tport:       tport,
		coreint:     coreint,
		printErrors: printErrors,
	}
}

// outputError writes out the given error.
func (b *bridge) outputError(err error) {
	if b.printErrors {
		logrus.Error(err)
	}
}

// CommandLoop is the main body of the bridge client. It waits for messages
// from the HCS, carries out the operations requested by the messages, responds
// to the HCS, and repeats.
func (b *bridge) CommandLoop() {
	if err := b.loop(); err != nil {
		b.outputError(err)
	}
}

func (b *bridge) loop() error {
	var err error
	b.commandConn, err = b.createAndConnectCommandConn()
	if err != nil {
		return err
	}
	defer b.commandConn.Close()

	for {
		// Messages from the HCS come as a header, which contains information
		// about the message (including its size), and a message body.
		// The header and body contain the operation to be performed, as well
		// as any information needed to perform the operation.
		message, header, err := readMessage(b.commandConn)
		if err != nil {
			return errors.Wrap(err, "failed to read a message from the HCS")
		}

		// Carry out the operations requested by the HCS.
		// Each operation has its own helper function, each of which returns a
		// response object.
		var response interface{}
		switch header.Type {
		case prot.ComputeSystemCreate_v1:
			utils.LogMsg("received from HCS: ComputeSystemCreate_v1")
			response, err = b.createContainer(message)
			if err != nil {
				b.outputError(err)
			}
		case prot.ComputeSystemExecuteProcess_v1:
			utils.LogMsg("received from HCS: ComputeSystemExecuteProcess_v1")
			response, err = b.execProcess(message)
			if err != nil {
				b.outputError(err)
			}
		case prot.ComputeSystemShutdownForced_v1:
			utils.LogMsg("received from HCS: ComputeSystemShutdownForced_v1")
			response, err = b.killContainer(message)
			if err != nil {
				b.outputError(err)
			}
		case prot.ComputeSystemShutdownGraceful_v1:
			utils.LogMsg("received from HCS: ComputeSystemShutdownGraceful_v1")
			response, err = b.shutdownContainer(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemTerminateProcess_v1:
			utils.LogMsg("received from HCS: ComputeSystemTerminateProcess_v1")
			response, err = b.terminateProcess(message)
			if err != nil {
				b.outputError(err)
			}
		case prot.ComputeSystemGetProperties_v1:
			utils.LogMsg("received from HCS: ComputeSystemGetProperties_v1")
			response, err = b.listProcesses(message)
			if err != nil {
				b.outputError(err)
			}
		case prot.ComputeSystemWaitForProcess_v1:
			utils.LogMsg("received from HCS: ComputeSystemWaitForProcess_v1")
			response, err = b.waitOnProcess(message, header)
			if err != nil {
				b.outputError(err)
			} else {
				// If no error occurred, don't respond until the process has
				// exited.
				response = nil
			}
		case prot.ComputeSystemResizeConsole_v1:
			utils.LogMsg("received from HCS: ComputeSystemResizeConsole_v1")
			response, err = b.resizeConsole(message)
			if err != nil {
				b.outputError(err)
			}
		case prot.ComputeSystemModifySettings_v1:
			utils.LogMsg("received from HCS: ComputeSystemModifySettings_v1")
			response, err = b.modifySettings(message)
			if err != nil {
				b.outputError(err)
			}
		default:
			// TODO: Should a response be returned in this case?
			b.outputError(errors.Errorf("received invalid header type code from HCS: 0x%x", header.Type))
		}

		// Set the error fields on the response if an error was encountered.
		if err != nil {
			switch response := response.(type) {
			case *prot.MessageResponseBase:
				b.setErrorForResponseBase(response, err)
			case *prot.ContainerCreateResponse:
				b.setErrorForResponseBase(response.MessageResponseBase, err)
			case *prot.ContainerExecuteProcessResponse:
				b.setErrorForResponseBase(response.MessageResponseBase, err)
			case *prot.ContainerWaitForProcessResponse:
				b.setErrorForResponseBase(response.MessageResponseBase, err)
			case *prot.ContainerGetPropertiesResponse:
				b.setErrorForResponseBase(response.MessageResponseBase, err)
			default:
				// TODO: Should this error be handled better?
				return errors.Errorf("invalid response type: %T", response)
			}
		}

		// Send a response to the HCS, but only if a response was specified.
		if response != nil {
			if err := b.sendResponse(response, header); err != nil {
				return errors.Wrap(err, "failed to send response to HCS")
			}
		}
	}
}

func (b *bridge) createContainer(message []byte) (*prot.ContainerCreateResponse, error) {
	response := &prot.ContainerCreateResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerCreate
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	// The request contains a JSON string field which is equivalent to a
	// CreateContainerInfo struct.
	var settings prot.VmHostedContainerSettings
	if err := json.Unmarshal([]byte(request.ContainerConfig), &settings); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for ContainerConfig \"%s\"", request.ContainerConfig)
	}

	id := request.ContainerId
	if err := b.coreint.CreateContainer(id, settings); err != nil {
		return response, err
	}

	exitHook := func(state oslayer.ProcessExitState) {
		if err := b.sendExitNotification(id, response.ActivityId, state); err != nil {
			b.outputError(err)
		}
	}
	if err := b.coreint.RegisterContainerExitHook(id, exitHook); err != nil {
		return response, err
	}

	response.SelectedProtocolVersion = prot.PV_V3
	return response, nil
}

func (b *bridge) execProcess(message []byte) (*prot.ContainerExecuteProcessResponse, error) {
	response := &prot.ContainerExecuteProcessResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerExecuteProcess
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	// The request contains a JSON string field which is equivalent to an
	// ExecuteProcessInfo struct.
	var params prot.ProcessParameters
	if err := json.Unmarshal([]byte(request.Settings.ProcessParameters), &params); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters)
	}

	// The same message type is used both to execute a process in a container,
	// and to execute a process in the utility VM itself. This field in the
	// message determines which operation is performed.
	if params.IsExternal {
		return b.runExternalProcess(message)
	}

	response.ActivityId = request.ActivityId
	id := request.ContainerId

	conns, err := createAndConnectStdio(b.tport, params, request.Settings.VsockStdioRelaySettings)
	if err != nil {
		return response, err
	}
	stdioSet := &core.StdioSet{
		In:  conns.In,
		Out: conns.Out,
		Err: conns.Err,
	}
	pid, err := b.coreint.ExecProcess(id, params, stdioSet)
	if err != nil {
		return response, err
	}

	// Close Connections on exit, but only for container processes without a
	// terminal.
	// TODO: Continue working on this so container processes without a terminal
	// properly write everything to stdio as well.
	if !params.EmulateConsole {
		exitHook := func(state oslayer.ProcessExitState) {
			if err := conns.Close(); err != nil {
				b.outputError(errors.Wrap(err, "failed to close Connections"))
			}
		}
		if err := b.coreint.RegisterProcessExitHook(pid, exitHook); err != nil {
			return response, err
		}
	}

	response.ProcessId = uint32(pid)
	return response, nil
}

func (b *bridge) killContainer(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.MessageBase
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	if err := b.coreint.SignalContainer(request.ContainerId, oslayer.SIGKILL); err != nil {
		return response, err
	}

	return response, nil
}

func (b *bridge) shutdownContainer(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.MessageBase
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	if err := b.coreint.SignalContainer(request.ContainerId, oslayer.SIGTERM); err != nil {
		return response, err
	}

	return response, nil
}

func (b *bridge) terminateProcess(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.ContainerTerminateProcess
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	if err := b.coreint.TerminateProcess(int(request.ProcessId)); err != nil {
		return response, err
	}

	return response, nil
}

func (b *bridge) listProcesses(message []byte) (*prot.ContainerGetPropertiesResponse, error) {
	response := &prot.ContainerGetPropertiesResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerGetProperties
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId
	id := request.ContainerId

	processes, err := b.coreint.ListProcesses(id)
	if err != nil {
		return response, err
	}

	processJson, err := json.Marshal(processes)
	if err != nil {
		return response, errors.Wrapf(err, "failed to marshal processes into JSON: %v", processes)
	}
	response.Properties = string(processJson)
	return response, nil
}

func (b *bridge) runExternalProcess(message []byte) (*prot.ContainerExecuteProcessResponse, error) {
	response := &prot.ContainerExecuteProcessResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerExecuteProcess
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	// The request contains a JSON string field which is equivalent to a
	// RunExternalProcessInfo struct.
	var params prot.ProcessParameters
	if err := json.Unmarshal([]byte(request.Settings.ProcessParameters), &params); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters)
	}

	conns, err := createAndConnectStdio(b.tport, params, request.Settings.VsockStdioRelaySettings)
	if err != nil {
		return response, err
	}
	stdioSet := &core.StdioSet{
		In:  conns.In,
		Out: conns.Out,
		Err: conns.Err,
	}
	pid, err := b.coreint.RunExternalProcess(params, stdioSet)
	if err != nil {
		return response, err
	}

	response.ProcessId = uint32(pid)
	return response, nil
}

func (b *bridge) waitOnProcess(message []byte, header *prot.MessageHeader) (*prot.ContainerWaitForProcessResponse, error) {
	response := &prot.ContainerWaitForProcessResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerWaitForProcess
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	exitHook := func(state oslayer.ProcessExitState) {
		response.ExitCode = uint32(state.ExitCode())
		if err := b.sendResponse(response, header); err != nil {
			b.outputError(errors.Wrapf(err, "failed to send process exit response \"%v\"", response))
		}
	}
	if err := b.coreint.RegisterProcessExitHook(int(request.ProcessId), exitHook); err != nil {
		return response, err
	}

	return response, nil
}

// resizeConsole is currently a nop until the functionality is implemented.
// TODO: Tests still need to be written when it's no longer a nop.
func (b *bridge) resizeConsole(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.ContainerResizeConsole
	if err := json.Unmarshal(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	// NOP

	return response, nil
}

func (b *bridge) modifySettings(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	request, err := prot.UnmarshalContainerModifySettings(message)
	if err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityId = request.ActivityId

	if err := b.coreint.ModifySettings(request.ContainerId, request.Request); err != nil {
		return response, err
	}

	return response, nil
}

// newResponseBase returns a MessageResponseBase with a default value.
func newResponseBase() *prot.MessageResponseBase {
	response := &prot.MessageResponseBase{
		ActivityId: "00000000-0000-0000-0000-000000000000",
	}
	return response
}

// setErrorForResponseBase modifies the passed-in MessageResponseBase to
// contain information pertaining to the given error.
func (b *bridge) setErrorForResponseBase(response *prot.MessageResponseBase, errForResponse error) {
	// stackTracer must be defined to access the stack trace of the error. I'm
	// not totally sure why it isn't just exported by the errors package.
	type stackTracer interface {
		StackTrace() errors.StackTrace
	}
	errorMessage := errForResponse.Error()
	fileName := ""
	lineNumber := -1
	functionName := ""
	if errForResponse, ok := errForResponse.(stackTracer); ok {
		frames := errForResponse.StackTrace()
		bottomFrame := frames[0]
		errorMessage = fmt.Sprintf("%+v", errForResponse)
		fileName = fmt.Sprintf("%s", bottomFrame)
		lineNumberStr := fmt.Sprintf("%d", bottomFrame)
		var err error
		lineNumber, err = strconv.Atoi(lineNumberStr)
		if err != nil {
			b.outputError(errors.Wrapf(err, "failed to parse \"%s\" as line number of error, using -1 instead", lineNumberStr))
			lineNumber = -1
		}
		functionName = fmt.Sprintf("%n", bottomFrame)
	}
	response.Result = -2147467259
	newRecord := prot.ErrorRecord{
		Result:       -2147467259,
		Message:      errorMessage,
		ModuleName:   "gcs",
		FileName:     fileName,
		Line:         uint32(lineNumber),
		FunctionName: functionName,
	}
	response.ErrorRecords = append(response.ErrorRecords, newRecord)
}

// sendResponse replies to an HCS request. It takes a response value and the
// header value from the request.
func (b *bridge) sendResponse(response interface{}, header *prot.MessageHeader) error {
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON for response \"%v\"", response)
	}
	//utils.LogMsgf("response sent to HCS (contents:%s)", responseBytes)
	utils.LogMsg("response sent to HCS")
	b.writeLock.Lock()
	defer b.writeLock.Unlock()
	if err := sendResponseBytes(b.commandConn, header.Type, header.Id, responseBytes); err != nil {
		return err
	}
	return nil
}

// sendExitNotification sends a notification to the HCS when the container with
// ID=id exits. An oslayer.ProcessExitState parameter is given with the exit
// state of the process.
func (b *bridge) sendExitNotification(id string, activityId string, state oslayer.ProcessExitState) error {
	result := state.ExitCode()
	notification := prot.ContainerNotification{
		MessageBase: &prot.MessageBase{
			ContainerId: id,
			ActivityId:  activityId,
		},
		Type:       prot.NT_UnexpectedExit, // TODO: Support different exit types.
		Operation:  prot.AO_None,
		Result:     int32(result),
		ResultInfo: "",
	}
	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON for notification \"%v\"", notification)
	}
	b.writeLock.Lock()
	defer b.writeLock.Unlock()
	if err := sendMessageBytes(b.commandConn, prot.ComputeSystemNotification_v1, 0, notificationBytes); err != nil {
		return err
	}
	return nil
}
