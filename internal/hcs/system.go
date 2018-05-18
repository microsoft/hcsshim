package hcs

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/hcsshim/internal/interop"
	"github.com/Microsoft/hcsshim/internal/schema1"
	"github.com/sirupsen/logrus"
)

var (
	defaultTimeout = time.Minute * 4
)

type System struct {
	handleLock     sync.RWMutex
	handle         hcsSystem
	ID             string
	callbackNumber uintptr
}

// createContainerAdditionalJSON is read from the environment at initialisation
// time. It allows an environment variable to define additional JSON which
// is merged in the CreateContainer call to HCS.
var createContainerAdditionalJSON string

func init() {
	createContainerAdditionalJSON = os.Getenv("HCSSHIM_CREATECONTAINER_ADDITIONALJSON")
}

// CreateContainer creates a new container with the given configuration but does not start it.
func CreateContainer(id string, c *schema1.ContainerConfig) (*System, error) {
	return createContainerWithJSON(id, c, "")
}

// CreateContainerWithJSON creates a new container with the given configuration but does not start it.
// It is identical to CreateContainer except that optional additional JSON can be merged before passing to HCS.
func CreateContainerWithJSON(id string, c *schema1.ContainerConfig, additionalJSON string) (*System, error) {
	return createContainerWithJSON(id, c, additionalJSON)
}

func createContainerWithJSON(id string, c *schema1.ContainerConfig, additionalJSON string) (*System, error) {
	operation := "CreateContainer"
	title := "HCSShim::" + operation

	container := &System{
		ID: id,
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	configuration := string(configurationb)
	logrus.Debugf(title+" id=%s config=%s", id, configuration)

	// Merge any additional JSON. Priority is given to what is passed in explicitly,
	// falling back to what's set in the environment.
	if additionalJSON == "" && createContainerAdditionalJSON != "" {
		additionalJSON = createContainerAdditionalJSON
	}
	if additionalJSON != "" {
		configurationMap := map[string]interface{}{}
		if err := json.Unmarshal([]byte(configuration), &configurationMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %s", configuration, err)
		}

		additionalMap := map[string]interface{}{}
		if err := json.Unmarshal([]byte(additionalJSON), &additionalMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %s", additionalJSON, err)
		}

		mergedMap := mergeMaps(additionalMap, configurationMap)
		mergedJSON, err := json.Marshal(mergedMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal merged configuration map %+v: %s", mergedMap, err)
		}

		configuration = string(mergedJSON)
		logrus.Debugf(title+" id=%s merged config=%s", id, configuration)
	}

	var (
		resultp  *uint16
		identity syscall.Handle
	)
	createError := hcsCreateComputeSystem(id, configuration, identity, &container.handle, &resultp)

	if createError == nil || IsPending(createError) {
		if err := container.registerCallback(); err != nil {
			// Terminate the container if it still exists. We're okay to ignore a failure here.
			container.Terminate()
			return nil, makeSystemError(container, operation, "", err)
		}
	}

	err = processAsyncHcsResult(createError, resultp, container.callbackNumber, hcsNotificationSystemCreateCompleted, &defaultTimeout)
	if err != nil {
		if err == ErrTimeout {
			// Terminate the container if it still exists. We're okay to ignore a failure here.
			container.Terminate()
		}
		return nil, makeSystemError(container, operation, configuration, err)
	}

	logrus.Debugf(title+" succeeded id=%s handle=%d", id, container.handle)
	return container, nil
}

// mergeMaps recursively merges map `fromMap` into map `ToMap`. Any pre-existing values
// in ToMap are overwritten. Values in fromMap are added to ToMap.
// From http://stackoverflow.com/questions/40491438/merging-two-json-strings-in-golang
func mergeMaps(fromMap, ToMap interface{}) interface{} {
	switch fromMap := fromMap.(type) {
	case map[string]interface{}:
		ToMap, ok := ToMap.(map[string]interface{})
		if !ok {
			return fromMap
		}
		for keyToMap, valueToMap := range ToMap {
			if valueFromMap, ok := fromMap[keyToMap]; ok {
				fromMap[keyToMap] = mergeMaps(valueFromMap, valueToMap)
			} else {
				fromMap[keyToMap] = valueToMap
			}
		}
	case nil:
		// merge(nil, map[string]interface{...}) -> map[string]interface{...}
		ToMap, ok := ToMap.(map[string]interface{})
		if ok {
			return ToMap
		}
	}
	return fromMap
}

// OpenContainer opens an existing container by ID.
func OpenContainer(id string) (*System, error) {
	operation := "OpenContainer"
	title := "HCSShim::" + operation
	logrus.Debugf(title+" id=%s", id)

	container := &System{
		ID: id,
	}

	var (
		handle  hcsSystem
		resultp *uint16
	)
	err := hcsOpenComputeSystem(id, &handle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, makeSystemError(container, operation, "", err)
	}

	container.handle = handle

	if err := container.registerCallback(); err != nil {
		return nil, makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s handle=%d", id, handle)
	return container, nil
}

// GetContainers gets a list of the containers on the system that match the query
func GetContainers(q schema1.ComputeSystemQuery) ([]schema1.ContainerProperties, error) {
	operation := "GetContainers"
	title := "HCSShim::" + operation

	queryb, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}

	query := string(queryb)
	logrus.Debugf(title+" query=%s", query)

	var (
		resultp         *uint16
		computeSystemsp *uint16
	)
	err = hcsEnumerateComputeSystems(query, &computeSystemsp, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, err
	}

	if computeSystemsp == nil {
		return nil, ErrUnexpectedValue
	}
	computeSystemsRaw := interop.ConvertAndFreeCoTaskMemBytes(computeSystemsp)
	computeSystems := []schema1.ContainerProperties{}
	if err := json.Unmarshal(computeSystemsRaw, &computeSystems); err != nil {
		return nil, err
	}

	logrus.Debugf(title + " succeeded")
	return computeSystems, nil
}

// Start synchronously starts the container.
func (container *System) Start() error {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "Start"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	if container.handle == 0 {
		return makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	var resultp *uint16
	err := hcsStartComputeSystem(container.handle, "", &resultp)
	err = processAsyncHcsResult(err, resultp, container.callbackNumber, hcsNotificationSystemStartCompleted, &defaultTimeout)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

// Shutdown requests a container shutdown, if IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (container *System) Shutdown() error {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "Shutdown"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	if container.handle == 0 {
		return makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	var resultp *uint16
	err := hcsShutdownComputeSystem(container.handle, "", &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

// Terminate requests a container terminate, if IsPending() on the error returned is true,
// it may not actually be shut down until Wait() succeeds.
func (container *System) Terminate() error {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "Terminate"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	if container.handle == 0 {
		return makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	var resultp *uint16
	err := hcsTerminateComputeSystem(container.handle, "", &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

// Wait synchronously waits for the container to shutdown or terminate.
func (container *System) Wait() error {
	operation := "Wait"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	err := waitForNotification(container.callbackNumber, hcsNotificationSystemExited, nil)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

// WaitTimeout synchronously waits for the container to terminate or the duration to elapse.
// If the timeout expires, IsTimeout(err) == true
func (container *System) WaitTimeout(timeout time.Duration) error {
	operation := "WaitTimeout"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	err := waitForNotification(container.callbackNumber, hcsNotificationSystemExited, &timeout)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

func (container *System) Properties(query string) (*schema1.ContainerProperties, error) {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	var (
		resultp     *uint16
		propertiesp *uint16
	)
	err := hcsGetComputeSystemProperties(container.handle, query, &propertiesp, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, err
	}

	if propertiesp == nil {
		return nil, ErrUnexpectedValue
	}
	propertiesRaw := interop.ConvertAndFreeCoTaskMemBytes(propertiesp)
	properties := &schema1.ContainerProperties{}
	if err := json.Unmarshal(propertiesRaw, properties); err != nil {
		return nil, err
	}
	return properties, nil
}

// Pause pauses the execution of the container. This feature is not enabled in TP5.
func (container *System) Pause() error {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "Pause"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	if container.handle == 0 {
		return makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	var resultp *uint16
	err := hcsPauseComputeSystem(container.handle, "", &resultp)
	err = processAsyncHcsResult(err, resultp, container.callbackNumber, hcsNotificationSystemPauseCompleted, &defaultTimeout)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

// Resume resumes the execution of the container. This feature is not enabled in TP5.
func (container *System) Resume() error {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "Resume"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	if container.handle == 0 {
		return makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	var resultp *uint16
	err := hcsResumeComputeSystem(container.handle, "", &resultp)
	err = processAsyncHcsResult(err, resultp, container.callbackNumber, hcsNotificationSystemResumeCompleted, &defaultTimeout)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

// CreateProcess launches a new process within the container.
func (container *System) CreateProcess(c *schema1.ProcessConfig) (*Process, error) {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "CreateProcess"
	title := "HCSShim::Container::" + operation
	var (
		processInfo   hcsProcessInformation
		processHandle hcsProcess
		resultp       *uint16
	)

	if container.handle == 0 {
		return nil, makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	// If we are not emulating a console, ignore any console size passed to us
	if !c.EmulateConsole {
		c.ConsoleSize[0] = 0
		c.ConsoleSize[1] = 0
	}

	configurationb, err := json.Marshal(c)
	if err != nil {
		return nil, makeSystemError(container, operation, "", err)
	}

	configuration := string(configurationb)
	logrus.Debugf(title+" id=%s config=%s", container.ID, configuration)

	err = hcsCreateProcess(container.handle, configuration, &processInfo, &processHandle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, makeSystemError(container, operation, configuration, err)
	}

	process := &Process{
		handle:    processHandle,
		processID: int(processInfo.ProcessId),
		system:    container,
		cachedPipes: &cachedPipes{
			stdIn:  processInfo.StdInput,
			stdOut: processInfo.StdOutput,
			stdErr: processInfo.StdError,
		},
	}

	if err := process.registerCallback(); err != nil {
		return nil, makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s processid=%d", container.ID, process.processID)
	return process, nil
}

// OpenProcess gets an interface to an existing process within the container.
func (container *System) OpenProcess(pid int) (*Process, error) {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "OpenProcess"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s, processid=%d", container.ID, pid)
	var (
		processHandle hcsProcess
		resultp       *uint16
	)

	if container.handle == 0 {
		return nil, makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	err := hcsOpenProcess(container.handle, uint32(pid), &processHandle, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return nil, makeSystemError(container, operation, "", err)
	}

	process := &Process{
		handle:    processHandle,
		processID: pid,
		system:    container,
	}

	if err := process.registerCallback(); err != nil {
		return nil, makeSystemError(container, operation, "", err)
	}

	logrus.Debugf(title+" succeeded id=%s processid=%s", container.ID, process.processID)
	return process, nil
}

// Close cleans up any state associated with the container but does not terminate or wait for it.
func (container *System) Close() error {
	container.handleLock.Lock()
	defer container.handleLock.Unlock()
	operation := "Close"
	title := "HCSShim::Container::" + operation
	logrus.Debugf(title+" id=%s", container.ID)

	// Don't double free this
	if container.handle == 0 {
		return nil
	}

	if err := container.unregisterCallback(); err != nil {
		return makeSystemError(container, operation, "", err)
	}

	if err := hcsCloseComputeSystem(container.handle); err != nil {
		return makeSystemError(container, operation, "", err)
	}

	container.handle = 0

	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}

func (container *System) registerCallback() error {
	context := &notifcationWatcherContext{
		channels: newChannels(),
	}

	callbackMapLock.Lock()
	callbackNumber := nextCallback
	nextCallback++
	callbackMap[callbackNumber] = context
	callbackMapLock.Unlock()

	var callbackHandle hcsCallback
	err := hcsRegisterComputeSystemCallback(container.handle, notificationWatcherCallback, callbackNumber, &callbackHandle)
	if err != nil {
		return err
	}
	context.handle = callbackHandle
	container.callbackNumber = callbackNumber

	return nil
}

func (container *System) unregisterCallback() error {
	callbackNumber := container.callbackNumber

	callbackMapLock.RLock()
	context := callbackMap[callbackNumber]
	callbackMapLock.RUnlock()

	if context == nil {
		return nil
	}

	handle := context.handle

	if handle == 0 {
		return nil
	}

	// hcsUnregisterComputeSystemCallback has its own syncronization
	// to wait for all callbacks to complete. We must NOT hold the callbackMapLock.
	err := hcsUnregisterComputeSystemCallback(handle)
	if err != nil {
		return err
	}

	closeChannels(context.channels)

	callbackMapLock.Lock()
	callbackMap[callbackNumber] = nil
	callbackMapLock.Unlock()

	handle = 0

	return nil
}

// Modifies the System by sending a request to HCS
func (container *System) Modify(config interface{}) error {
	container.handleLock.RLock()
	defer container.handleLock.RUnlock()
	operation := "Modify"
	title := "HCSShim::Container::" + operation

	if container.handle == 0 {
		return makeSystemError(container, operation, "", ErrAlreadyClosed)
	}

	requestJSON, err := json.Marshal(config)
	if err != nil {
		return err
	}

	requestString := string(requestJSON)
	logrus.Debugf(title+" id=%s request=%s", container.ID, requestString)

	var resultp *uint16
	err = hcsModifyComputeSystem(container.handle, requestString, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		return makeSystemError(container, operation, "", err)
	}
	logrus.Debugf(title+" succeeded id=%s", container.ID)
	return nil
}
