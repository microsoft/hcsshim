// Package bridge defines the bridge struct, which implements the control loop
// and functions of the GCS's bridge client.
package bridge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/Microsoft/opengcs/service/gcs/core"
	gcserr "github.com/Microsoft/opengcs/service/gcs/errors"
	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

// bridge defines the bridge client in the GCS.
type bridge struct {
	// tport is the Transport interface used by the bridge.
	tport transport.Transport

	// coreint is the Core interface used by the bridge.
	coreint core.Core

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
func NewBridge(tport transport.Transport, coreint core.Core) *bridge {
	return &bridge{
		tport:   tport,
		coreint: coreint,
	}
}

// CommandLoop is the main body of the bridge client. It waits for messages
// from the HCS, carries out the operations requested by the messages, responds
// to the HCS, and repeats.
func (b *bridge) CommandLoop() {
	if err := b.loop(); err != nil {
		logrus.Error(err)
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
		case prot.ComputeSystemCreateV1:
			logrus.Info("received from HCS: ComputeSystemCreateV1")
			response, err = b.createContainer(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemExecuteProcessV1:
			logrus.Info("received from HCS: ComputeSystemExecuteProcessV1")
			response, err = b.execProcess(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemShutdownForcedV1:
			logrus.Info("received from HCS: ComputeSystemShutdownForcedV1")
			response, err = b.killContainer(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemShutdownGracefulV1:
			logrus.Info("received from HCS: ComputeSystemShutdownGracefulV1")
			response, err = b.shutdownContainer(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemSignalProcessV1:
			logrus.Info("received from HCS: ComputeSystemSignalProcessV1")
			response, err = b.signalProcess(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemGetPropertiesV1:
			logrus.Info("received from HCS: ComputeSystemGetPropertiesV1")
			response, err = b.listProcesses(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemWaitForProcessV1:
			logrus.Info("received from HCS: ComputeSystemWaitForProcessV1")
			response, err = b.waitOnProcess(message, header)
			if err != nil {
				logrus.Error(err)
			} else {
				// If no error occurred, don't respond until the process has
				// exited.
				response = nil
			}
		case prot.ComputeSystemResizeConsoleV1:
			logrus.Info("received from HCS: ComputeSystemResizeConsoleV1")
			response, err = b.resizeConsole(message)
			if err != nil {
				logrus.Error(err)
			}
		case prot.ComputeSystemModifySettingsV1:
			logrus.Info("received from HCS: ComputeSystemModifySettingsV1")
			response, err = b.modifySettings(message)
			if err != nil {
				logrus.Error(err)
			}
		default:
			// TODO: Should a response be returned in this case?
			logrus.Error(errors.Errorf("received invalid header type code from HCS: 0x%x", header.Type))
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
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON in message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	// The request contains a JSON string field which is equivalent to a
	// CreateContainerInfo struct.
	var settings prot.VMHostedContainerSettings
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.ContainerConfig), &settings); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for ContainerConfig \"%s\"", request.ContainerConfig)
	}

	id := request.ContainerID
	if err := b.coreint.CreateContainer(id, settings); err != nil {
		return response, err
	}

	exitHook := func(state oslayer.ProcessExitState) {
		if err := b.sendExitNotification(id, response.ActivityID, state); err != nil {
			logrus.Error(err)
		}
	}
	if err := b.coreint.RegisterContainerExitHook(id, exitHook); err != nil {
		return response, err
	}

	response.SelectedProtocolVersion = prot.PvV3
	return response, nil
}

func (b *bridge) execProcess(message []byte) (*prot.ContainerExecuteProcessResponse, error) {
	response := &prot.ContainerExecuteProcessResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerExecuteProcess
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	// The request contains a JSON string field which is equivalent to an
	// ExecuteProcessInfo struct.
	var params prot.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.Settings.ProcessParameters), &params); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters)
	}

	// The same message type is used both to execute a process in a container,
	// and to execute a process in the utility VM itself. This field in the
	// message determines which operation is performed.
	if params.IsExternal {
		return b.runExternalProcess(message)
	}

	response.ActivityID = request.ActivityID
	id := request.ContainerID

	stdioSet, err := connectStdio(b.tport, params, request.Settings.VsockStdioRelaySettings)
	if err != nil {
		return response, err
	}
	pid, err := b.coreint.ExecProcess(id, params, stdioSet)
	if err != nil {
		stdioSet.Close() // stdioSet will be eventually closed by coreint on success
		return response, err
	}

	response.ProcessID = uint32(pid)
	return response, nil
}

func (b *bridge) killContainer(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	if err := b.coreint.SignalContainer(request.ContainerID, oslayer.SIGKILL); err != nil {
		return response, err
	}

	return response, nil
}

func (b *bridge) shutdownContainer(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.MessageBase
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	if err := b.coreint.SignalContainer(request.ContainerID, oslayer.SIGTERM); err != nil {
		return response, err
	}

	return response, nil
}

func (b *bridge) signalProcess(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.ContainerSignalProcess
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	if err := b.coreint.SignalProcess(int(request.ProcessID), request.Options); err != nil {
		return response, err
	}

	return response, nil
}

func (b *bridge) listProcesses(message []byte) (*prot.ContainerGetPropertiesResponse, error) {
	response := &prot.ContainerGetPropertiesResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerGetProperties
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID
	id := request.ContainerID

	processes, err := b.coreint.ListProcesses(id)
	if err != nil {
		return response, err
	}

	processJSON, err := json.Marshal(processes)
	if err != nil {
		return response, errors.Wrapf(err, "failed to marshal processes into JSON: %v", processes)
	}
	response.Properties = string(processJSON)
	return response, nil
}

func (b *bridge) runExternalProcess(message []byte) (*prot.ContainerExecuteProcessResponse, error) {
	response := &prot.ContainerExecuteProcessResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerExecuteProcess
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	// The request contains a JSON string field which is equivalent to a
	// RunExternalProcessInfo struct.
	var params prot.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult([]byte(request.Settings.ProcessParameters), &params); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for ProcessParameters \"%s\"", request.Settings.ProcessParameters)
	}

	stdioSet, err := connectStdio(b.tport, params, request.Settings.VsockStdioRelaySettings)
	if err != nil {
		return response, err
	}
	pid, err := b.coreint.RunExternalProcess(params, stdioSet)
	if err != nil {
		stdioSet.Close() // stdioSet will be eventually closed by coreint on success
		return response, err
	}

	response.ProcessID = uint32(pid)
	return response, nil
}

func (b *bridge) waitOnProcess(message []byte, header *prot.MessageHeader) (*prot.ContainerWaitForProcessResponse, error) {
	response := &prot.ContainerWaitForProcessResponse{MessageResponseBase: newResponseBase()}
	var request prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	exitHook := func(state oslayer.ProcessExitState) {
		response.ExitCode = uint32(state.ExitCode())
		if err := b.sendResponse(response, header); err != nil {
			logrus.Error(errors.Wrapf(err, "failed to send process exit response \"%v\"", response))
		}
	}
	if err := b.coreint.RegisterProcessExitHook(int(request.ProcessID), exitHook); err != nil {
		return response, err
	}

	return response, nil
}

// resizeConsole is currently a nop until the functionality is implemented.
// TODO: Tests still need to be written when it's no longer a nop.
func (b *bridge) resizeConsole(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()
	var request prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(message, &request); err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}
	response.ActivityID = request.ActivityID

	// NOP

	return response, nil
}

func (b *bridge) modifySettings(message []byte) (*prot.MessageResponseBase, error) {
	response := newResponseBase()

	// We do a high level deserialization of just the base message (container/activity id) as early as possible
	// so that we can add tracking even if deserialization fails for a lower level later.
	var base prot.MessageBase
	err := commonutils.UnmarshalJSONWithHresult(message, &base)
	if err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}

	response.ActivityID = base.ActivityID

	request, err := prot.UnmarshalContainerModifySettings(message)
	if err != nil {
		return response, errors.Wrapf(err, "failed to unmarshal JSON for message \"%s\"", message)
	}

	if err := b.coreint.ModifySettings(request.ContainerID, request.Request); err != nil {
		return response, err
	}

	return response, nil
}

// newResponseBase returns a MessageResponseBase with a default value.
func newResponseBase() *prot.MessageResponseBase {
	response := &prot.MessageResponseBase{
		ActivityID: "00000000-0000-0000-0000-000000000000",
	}
	return response
}

// setErrorForResponseBase modifies the passed-in MessageResponseBase to
// contain information pertaining to the given error.
func (b *bridge) setErrorForResponseBase(response *prot.MessageResponseBase, errForResponse error) {
	errorMessage := errForResponse.Error()
	fileName := ""
	lineNumber := -1
	functionName := ""
	if serr, ok := errForResponse.(gcserr.StackTracer); ok {
		frames := serr.StackTrace()
		bottomFrame := frames[0]
		errorMessage = fmt.Sprintf("%+v", serr)
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
	//logrus.Infof("response sent to HCS (contents:%s)", responseBytes)
	logrus.Info("response sent to HCS")
	b.writeLock.Lock()
	defer b.writeLock.Unlock()
	if err := sendResponseBytes(b.commandConn, header.Type, header.ID, responseBytes); err != nil {
		return err
	}
	return nil
}

// sendExitNotification sends a notification to the HCS when the container with
// ID=id exits. An oslayer.ProcessExitState parameter is given with the exit
// state of the process.
func (b *bridge) sendExitNotification(id string, activityID string, state oslayer.ProcessExitState) error {
	result := state.ExitCode()
	notification := prot.ContainerNotification{
		MessageBase: &prot.MessageBase{
			ContainerID: id,
			ActivityID:  activityID,
		},
		Type:       prot.NtUnexpectedExit, // TODO: Support different exit types.
		Operation:  prot.AoNone,
		Result:     int32(result),
		ResultInfo: "",
	}
	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal JSON for notification \"%v\"", notification)
	}
	b.writeLock.Lock()
	defer b.writeLock.Unlock()
	if err := sendMessageBytes(b.commandConn, prot.ComputeSystemNotificationV1, 0, notificationBytes); err != nil {
		return err
	}
	return nil
}
