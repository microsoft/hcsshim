//go:build windows
// +build windows

package bridge

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	hcsschema "github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/hcs/schema2/resourcepaths"
	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/cmd/gcs-sidecar/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/hcn"
)

// Current intent of these handler functions is to call the security policy
// enforcement code as needed and return nil if the operation is allowed.
// Else error is returned.
// Also, currently, the caller of this function is forwarding the request
// to inbox GCS if handler returns nil. This is because we want to process
// response from inbox GCS asynchronously.
// TODO: The caller, that is hcsshim, starts a 30 second timer and if response
// is not got by then, bridge is killed. Should we track responses from gcs by
// time in sidecar too? Maybe not.
func (b *Bridge) createContainer(req *request) error {
	var r containerCreate
	var containerConfig json.RawMessage
	r.ContainerConfig.Value = &containerConfig
	if err := json.Unmarshal(req.message, &r); err != nil {
		log.Printf("failed to unmarshal rpcCreate: %v", req)
		// TODO: Send valid error response back to the sender before closing bridge
		return fmt.Errorf("failed to unmarshal rpcCreate: %v", req)
	}

	var err error
	var uvmConfig uvmConfig
	var hostedSystemConfig hcsschema.HostedSystem
	if err = json.Unmarshal(containerConfig, &uvmConfig); err == nil {
		systemType := uvmConfig.SystemType
		timeZoneInformation := uvmConfig.TimeZoneInformation
		log.Printf("rpcCreate: \n ContainerCreate{ requestBase: %v, uvmConfig: {systemType: %v, timeZoneInformation: %v}}", r.requestBase, systemType, timeZoneInformation)
		// err =  call policyEnforcer
		err = nil
	} else if err = json.Unmarshal(containerConfig, &hostedSystemConfig); err == nil {
		schemaVersion := hostedSystemConfig.SchemaVersion
		container := hostedSystemConfig.Container
		log.Printf("rpcCreate: \n ContainerCreate{ requestBase: %v, ContainerConfig: {schemaVersion: %v, container: %v}}", r.requestBase, schemaVersion, container)
		// err = call policyEnforcer
		err = nil
	} else {
		log.Printf("createContainer: invalid containerConfig type. Request: %v", req)
		// TODO: Send valid error response back to the sender before closing bridge
		err = fmt.Errorf("createContainer: invalid containerConfig type. Request: %v", r)
	}

	return err
}

func (b *Bridge) startContainer(req *request) error {
	var r requestBase
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcStart: %v", req)
	}
	log.Printf("rpcStart: \n requestBase: %v", r)

	return nil
}

func (b *Bridge) shutdownGraceful(req *request) error {
	var r requestBase
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcShutdownGraceful: %v", req)
	}
	log.Printf("rpcShutdownGraceful: \n requestBase: %v", r)

	/*
		containerdID := r.ContainerdID
		b.securityPolicyEnforcer.EnforceShutdownContainerPolicy(ctx, containerID)
		if err != nil {
			return fmt.Errorf("rpcShudownGraceful operation not allowed: %v", err)
		}
	*/
	return nil
}

func (b *Bridge) shutdownForced(req *request) error {
	var r requestBase
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcShutdownForced: %v", req)
	}
	log.Printf("rpcShutdownForced: \n requestBase: %v", r)

	/*
		containerdID := r.ContainerdID
		b.securityPolicyEnforcer.EnforceShutdownContainerPolicy(ctx, containerID)
		if err != nil {
			return fmt.Errorf("rpcShudownGraceful operation not allowed: %v", err)
		}
	*/

	return nil
}

func (b *Bridge) executeProcess(req *request) error {
	var r containerExecuteProcess
	var processParamSettings json.RawMessage
	r.Settings.ProcessParameters.Value = &processParamSettings
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcExecuteProcess: %v", req)
	}
	containerID := r.requestBase.ContainerID
	stdioRelaySettings := r.Settings.StdioRelaySettings
	vsockStdioRelaySettings := r.Settings.VsockStdioRelaySettings

	var err error
	var processParams hcsschema.ProcessParameters
	if err = json.Unmarshal(processParamSettings, &processParams); err != nil {
		log.Printf("rpcExecProcess: invalid params type for request %v", r.Settings)
		return fmt.Errorf("rpcExecProcess: invalid params type for request %v", r.Settings)
	}

	log.Printf("rpcExecProcess: \n containerID: %v, schema1.ProcessParameters{ params: %v, stdioRelaySettings: %v, vsockStdioRelaySettings: %v }", containerID, processParams, stdioRelaySettings, vsockStdioRelaySettings)
	// err = call policy enforcer
	return err
}

func (b *Bridge) waitForProcess(req *request) error {
	var r containerWaitForProcess
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcShutdownForced: %v", req)
	}
	log.Printf("rpcWaitForProcess: \n containerWaitForProcess{ requestBase: %v, processID: %v, timeoutInMs: %v }", r.requestBase, r.ProcessID, r.TimeoutInMs)

	// waitForProcess does not have enforcer in clcow, why?
	return nil
}

func (b *Bridge) signalProcess(req *request) error {
	var r containerSignalProcess
	var rawOpts json.RawMessage
	r.Options = &rawOpts
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcSignalProcess: %v", req)
	}

	log.Printf("rpcSignalProcess: request %v", r)

	var err error
	var wcowOptions guestresource.SignalProcessOptionsWCOW
	if rawOpts == nil {
		return nil
	} else if err = json.Unmarshal(rawOpts, &wcowOptions); err != nil {
		log.Printf("rpcSignalProcess: invalid Options type for request %v", r)
		return fmt.Errorf("rpcSignalProcess: invalid Options type for request %v", r)
	}
	log.Printf("rpcSignalProcess: \n containerSignalProcess{ requestBase: %v, processID: %v, Options: %v }", r.requestBase, r.ProcessID, wcowOptions)

	// calling policy enforcer
	err = signalProcess(r.ContainerID, r.ProcessID, wcowOptions.Signal)
	if err != nil {
		return fmt.Errorf("waitForProcess not allowed due to policy")
	}

	return nil
}

func (b *Bridge) resizeConsole(req *request) error {
	var r containerResizeConsole
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcSignalProcess: %v", req)
	}
	log.Printf("rpcResizeConsole: \n containerResizeConsole{ requestBase: %v, processID: %v, height: %v, width: %v }", r.requestBase, r.ProcessID, r.Height, r.Width)

	err := resizeConsole(r.ContainerID, r.Height, r.Width)
	if err != nil {
		return fmt.Errorf("waitForProcess not allowed due to policy")
	}

	return nil
}

func (b *Bridge) getProperties(req *request) error {
	// TODO: This has containerGetProperties and containerGetPropertiesV2. Need to find a way to differentiate!
	/*
		var r containerGetProperties
		if err := json.Unmarshal(req.message, &r); err != nil {
			return fmt.Errorf("failed to unmarshal rpcSignalProcess: %v", req)
		}
	*/
	return nil
}

func isSpecialResourcePaths(resourcePath string, settings interface{}) bool {
	if strings.HasPrefix(resourcePath, resourcepaths.HvSocketConfigResourcePrefix) {
		sid := strings.TrimPrefix(resourcePath, resourcepaths.HvSocketConfigResourcePrefix)
		doc := settings.(*hcsschema.HvSocketServiceConfig)
		log.Printf(", sid: %v, HvSocketServiceConfig{ %v } \n", sid, doc)
		return true
	} else if strings.HasPrefix(resourcePath, resourcepaths.NetworkResourcePrefix) {
		id := strings.TrimPrefix(resourcePath, resourcepaths.NetworkResourcePrefix)
		settings := settings.(*hcsschema.NetworkAdapter)
		log.Printf(", sid: %v, NetworkAdapter{ %v } \n", id, settings)
		return true
	} else if strings.HasPrefix(resourcePath, resourcepaths.SCSIResourcePrefix) {
		var controller string
		var lun string
		if _, err := fmt.Sscanf(resourcePath, resourcepaths.SCSIResourceFormat, &controller, &lun); err != nil {
			log.Printf("Invalid SCSIResourceFormat %v", resourcePath)
			return false
		} else {
			log.Printf(", controller: %v, lun{ %v } \n", controller, lun)
		}
		return true
	}
	// if we reached here, request is invalid
	return false
}

func unMarshalAndModifySettings(modifySettingsRequest *hcsschema.ModifySettingRequest, requestRawSettings *json.RawMessage) error {
	if modifySettingsRequest.ResourcePath != "" {
		reqType := modifySettingsRequest.RequestType
		resourcePath := modifySettingsRequest.ResourcePath

		log.Printf("rpcModifySettings: ModifySettingRequest { RequestType: %v \n, ResourcePath: %v", reqType, resourcePath)

		switch resourcePath {
		case resourcepaths.SiloMappedDirectoryResourcePath:
			mappedDirectory := modifySettingsRequest.Settings.(*hcsschema.MappedDirectory)
			// TODO: check for Settings to be nil as in some examples
			log.Printf(", mappedDirectory: %v \n", mappedDirectory)
		case resourcepaths.SiloMemoryResourcePath:
			memoryLimit := modifySettingsRequest.Settings.(*uint64)
			log.Printf(", memoryLimit: %v \n", memoryLimit)
		case resourcepaths.SiloProcessorResourcePath:
			processor := modifySettingsRequest.Settings.(*hcsschema.Processor)
			log.Printf(", processor: %v \n", processor)
		case resourcepaths.CPUGroupResourcePath:
			cpuGroup := modifySettingsRequest.Settings.(*hcsschema.CpuGroup)
			log.Printf(", cpuGroup: %v \n", cpuGroup)
		case resourcepaths.CPULimitsResourcePath:
			processorLimits := modifySettingsRequest.Settings.(*hcsschema.ProcessorLimits)
			log.Printf(", processorLimits: %v \n", processorLimits)
		case resourcepaths.MemoryResourcePath:
			actualMemory := modifySettingsRequest.Settings.(*uint64)
			log.Printf(", actualMemory: %v \n", actualMemory)
		case resourcepaths.VSMBShareResourcePath:
			virtualSmbShareSettings := modifySettingsRequest.Settings.(*hcsschema.VirtualSmbShare)
			log.Printf(", VirtualSmbShare: %v \n", virtualSmbShareSettings)
		// TODO: Plan9 is only for LCOW right?
		// case resourcepaths.Plan9ShareResourcePath:
		//	plat9ShareSettings := modifyRequest.Settings.(*hcsschema.Plan9Share)
		//	log.Printf(", Plan9Share: %v \n", plat9ShareSettings)

		// TODO: Does following apply for cwcow?
		// case resourcepaths.VirtualPCIResourceFormat
		// case resourcepaths.VPMemControllerResourceFormat
		default:
			// Handle cases of HvSocketConfigResourcePrefix, NetworkResourceFormatetc as they have data values in resourcePath string
			if !isSpecialResourcePaths(resourcePath, modifySettingsRequest.Settings) {
				return fmt.Errorf("invalid rpcModifySettings resourcePath %v", resourcePath)
			}
		}
	}

	if modifySettingsRequest.GuestRequest != nil {
		var guestModificationRequest guestrequest.ModificationRequest
		if err := json.Unmarshal(*requestRawSettings, &guestModificationRequest); err != nil {
			log.Printf("invalid modifySettingsRequest.guestRequest")
			return fmt.Errorf("invalid modifySettingsRequest.guestRequest")
		}
		// modifyRequest.GuestRequest != nil

		guestResourceType := guestModificationRequest.ResourceType
		guestRequestType := guestModificationRequest.RequestType

		log.Printf("rpcModifySettings: guestRequest.ModificationRequest { resourceType: %v \n, requestType: %v", guestResourceType, guestRequestType)

		switch guestResourceType {
		case guestresource.ResourceTypeCombinedLayers:
			settings := guestModificationRequest.Settings.(*guestresource.WCOWCombinedLayers)
			log.Printf(", WCOWCombinedLayers {ContainerRootPath: %v, Layers: %v, ScratchPath: %v} \n", settings.ContainerRootPath, settings.Layers, settings.ScratchPath)
		case guestresource.ResourceTypeNetworkNamespace:
			settings := guestModificationRequest.Settings.(*hcn.HostComputeNamespace)
			log.Printf(", HostComputeNamespaces { %v} \n", settings)
		case guestresource.ResourceTypeNetwork:
			// following valid only for osversion.Build() >= osversion.RS5
			// since Cwcow is available only for latest versions this is ok
			settings := guestModificationRequest.Settings.(*guestrequest.NetworkModifyRequest)
			log.Printf(", NetworkModifyRequest { %v} \n", settings)
		case guestresource.ResourceTypeMappedVirtualDisk:
			wcowMappedVirtualDisk := guestModificationRequest.Settings.(*guestresource.WCOWMappedVirtualDisk)
			log.Printf(", wcowMappedVirtualDisk { %v} \n", wcowMappedVirtualDisk)
		// TODO need a case similar to guestresource.ResourceTypeSecurityPolicy of lcow?
		// case guestresource.ResourceTypeSecurityPolicy:
		case guestresource.ResourceTypeHvSocket:
			hvSocketAddress := guestModificationRequest.Settings.(*hcsschema.HvSocketAddress)
			log.Printf(", hvSocketAddress { %v} \n", hvSocketAddress)
		default:
			isSpecialGuestRequests(string(guestResourceType), guestModificationRequest.Settings)
			// invalid
		}
	}

	// call policy enforcer
	return nil
}

func (b *Bridge) modifySettings(req *request) error {
	var r containerModifySettings
	var requestRawSettings json.RawMessage
	r.Request = &requestRawSettings
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcModifySettings: %v", req)
	}

	var err error
	var modifySettingsRequest hcsschema.ModifySettingRequest
	if err = json.Unmarshal(requestRawSettings, &modifySettingsRequest); err != nil {
		log.Printf("invalid rpcModifySettings request %v", r)
		return fmt.Errorf("invalid rpcModifySettings request %v", r)
	}

	return unMarshalAndModifySettings(&modifySettingsRequest, &requestRawSettings)
}

func isSpecialGuestRequests(guestResourceType string, settings interface{}) bool {
	if strings.HasPrefix(guestResourceType, resourcepaths.MappedPipeResourcePrefix) {
		hostPath := strings.TrimPrefix(guestResourceType, resourcepaths.MappedPipeResourcePrefix)
		log.Printf(", hostPath: %v \n", hostPath)
		return true
	}
	// if we reached here, request is invalid
	return false
}

func (b *Bridge) sendMessage(typ msgType, id int64, msg []byte) {
	var h [hdrSize]byte
	binary.LittleEndian.PutUint32(h[:], uint32(typ))
	binary.LittleEndian.PutUint32(h[4:], uint32(len(msg)+16))
	binary.LittleEndian.PutUint64(h[8:], uint64(id))

	b.sendToShimCh <- request{
		header:  h,
		message: msg,
	}
	// time.Sleep(2 * time.Second)
}

func (b *Bridge) negotiateProtocol(req *request) error {
	var r negotiateProtocolRequest
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcNegotiateProtocol: %v", req)
	}
	log.Printf("rpcNegotiateProtocol: negotiateProtocolRequest{ requestBase %v, MinVersion: %v, MaxVersion: %v }", r.requestBase, r.MinimumVersion, r.MaximumVersion)

	return nil
}

func (b *Bridge) dumpStacks(req *request) error {
	var r dumpStacksRequest
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcStart: %v", req)
	}
	log.Printf("rpcDumpStacks: \n requestBase: %v", r.requestBase)

	return nil
}

func (b *Bridge) deleteContainerState(req *request) error {
	var r deleteContainerStateRequest
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcStart: %v", req)
	}
	log.Printf("rpcDeleteContainerRequest: \n requestBase: %v", r.requestBase)

	return nil
}

func (b *Bridge) updateContainer(req *request) error {
	// No callers in the code for rpcUpdateContainer
	return nil
}

func (b *Bridge) lifecycleNotification(req *request) error {
	// No callers in the code for rpcLifecycleNotification
	return nil
}
