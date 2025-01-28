//go:build windows
// +build windows

package bridge

import (
	"context"
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
// Also, these handler functions decide if request needs to be forwarded
// to inbox GCS or not. Request is forwarded asynchronously.
// TODO: The caller, that is hcsshim, starts a 30 second timer and if response
// is not got by then, bridge is killed. Should we track responses from gcs by
// time in sidecar too? Maybe not.
func (b *Bridge) createContainer(req *request) error {
	var err error
	err = nil
	var r containerCreate
	var containerConfig json.RawMessage
	r.ContainerConfig.Value = &containerConfig
	if err = json.Unmarshal(req.message, &r); err != nil {
		log.Printf("failed to unmarshal rpcCreate: %v", req)
		// TODO: Send valid error response back to the sender before closing bridge
		return fmt.Errorf("failed to unmarshal rpcCreate: %v", req)
	}

	var uvmConfig uvmConfig
	var hostedSystemConfig hcsschema.HostedSystem
	if err = json.Unmarshal(containerConfig, &uvmConfig); err == nil {
		systemType := uvmConfig.SystemType
		timeZoneInformation := uvmConfig.TimeZoneInformation
		log.Printf("rpcCreate: \n ContainerCreate{ requestBase: %v, uvmConfig: {systemType: %v, timeZoneInformation: %v}}", r.requestBase, systemType, timeZoneInformation)
		// TODO: call policy enforcement points once ready
		// err = call policyEnforcer
		// return on err
	} else if err = json.Unmarshal(containerConfig, &hostedSystemConfig); err == nil {
		schemaVersion := hostedSystemConfig.SchemaVersion
		container := hostedSystemConfig.Container
		log.Printf("rpcCreate: \n ContainerCreate{ requestBase: %v, ContainerConfig: {schemaVersion: %v, container: %v}}", r.requestBase, schemaVersion, container)
		// TODO: call policy enforcement points once ready
		// err = call policyEnforcer
		// return on err
	} else {
		log.Printf("createContainer: invalid containerConfig type. Request: %v", req)
		// TODO: Send valid error response back to the sender before closing bridge
		return fmt.Errorf("createContainer: invalid containerConfig type. Request: %v", r)
	}

	// If we've reached here, means the policy has allowed operation.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return err
}

func (b *Bridge) startContainer(req *request) error {
	var r requestBase
	var err error
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcStart: %v", req)
	}
	log.Printf("rpcStart: \n requestBase: %v", r)

	// TODO: call policy enforcement points once ready
	// err = call policyEnforcer
	// return on err

	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) shutdownGraceful(req *request) error {
	var r requestBase
	var err error
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcShutdownGraceful: %v", req)
	}
	log.Printf("rpcShutdownGraceful: \n requestBase: %v", r)
	/*
		containerdID := r.ContainerdID
		b.PolicyEnforcer.EnforceShutdownContainerPolicy(ctx, containerID)
		if err != nil {
			return fmt.Errorf("rpcShudownGraceful operation not allowed: %v", err)
		}
	*/
	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) shutdownForced(req *request) error {
	var r requestBase
	var err error
	err = nil
	if err = json.Unmarshal(req.message, &r); err != nil {
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

	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) executeProcess(req *request) error {
	var r containerExecuteProcess
	var processParamSettings json.RawMessage
	var err error
	err = nil
	r.Settings.ProcessParameters.Value = &processParamSettings
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcExecuteProcess: %v", req)
	}
	containerID := r.requestBase.ContainerID
	stdioRelaySettings := r.Settings.StdioRelaySettings
	vsockStdioRelaySettings := r.Settings.VsockStdioRelaySettings

	var processParams hcsschema.ProcessParameters
	if err = json.Unmarshal(processParamSettings, &processParams); err != nil {
		log.Printf("rpcExecProcess: invalid params type for request %v", r.Settings)
		return fmt.Errorf("rpcExecProcess: invalid params type for request %v", r.Settings)
	}

	log.Printf("rpcExecProcess: \n containerID: %v, schema1.ProcessParameters{ params: %v, stdioRelaySettings: %v, vsockStdioRelaySettings: %v }", containerID, processParams, stdioRelaySettings, vsockStdioRelaySettings)
	// err = call policy enforcer

	b.sendToGCSChan <- *req

	return err
}

func (b *Bridge) waitForProcess(req *request) error {
	var r containerWaitForProcess
	var err error
	err = nil
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal waitForProcess: %v", req)
	}
	log.Printf("rpcWaitForProcess: \n containerWaitForProcess{ requestBase: %v, processID: %v, timeoutInMs: %v }", r.requestBase, r.ProcessID, r.TimeoutInMs)

	// waitForProcess does not have enforcer in clcow, why?

	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) signalProcess(req *request) error {
	var err error
	var r containerSignalProcess
	var rawOpts json.RawMessage
	r.Options = &rawOpts
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcSignalProcess: %v", req)
	}

	log.Printf("rpcSignalProcess: request %v", r)

	var wcowOptions guestresource.SignalProcessOptionsWCOW
	if rawOpts == nil {
		b.sendToGCSChan <- *req
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

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) resizeConsole(req *request) error {
	var r containerResizeConsole
	var err error
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcSignalProcess: %v", req)
	}
	log.Printf("rpcResizeConsole: \n containerResizeConsole{ requestBase: %v, processID: %v, height: %v, width: %v }", r.requestBase, r.ProcessID, r.Height, r.Width)

	err = resizeConsole(r.ContainerID, r.Height, r.Width)
	if err != nil {
		return fmt.Errorf("waitForProcess not allowed due to policy")
	}

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

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

func isSpecialResourcePaths(resourcePath string, rawGuestRequest json.RawMessage) bool {
	if strings.HasPrefix(resourcePath, resourcepaths.HvSocketConfigResourcePrefix) {
		sid := strings.TrimPrefix(resourcePath, resourcepaths.HvSocketConfigResourcePrefix)
		doc := &hcsschema.HvSocketServiceConfig{}

		if err := json.Unmarshal(rawGuestRequest, &doc); err != nil {
			log.Printf("invalid rpcModifySettings request %v", rawGuestRequest)
			return false
			//fmt.Errorf("invalid rpcModifySettings request %v", r)
		}

		log.Printf(", sid: %v, HvSocketServiceConfig{ %v } \n", sid, doc)
		return true
	} else if strings.HasPrefix(resourcePath, resourcepaths.NetworkResourcePrefix) {
		id := strings.TrimPrefix(resourcePath, resourcepaths.NetworkResourcePrefix)
		settings := &hcsschema.NetworkAdapter{}
		if err := json.Unmarshal(rawGuestRequest, &settings); err != nil {
			log.Printf("invalid rpcModifySettings request %v", rawGuestRequest)
			return false
			//fmt.Errorf("invalid rpcModifySettings request %v", r)
		}

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

func (b *Bridge) unMarshalAndModifySettings(req *request) error {
	// skipSendToGCS := false
	// var err error
	// err = nil
	var r containerModifySettings
	var requestRawSettings json.RawMessage
	r.Request = &requestRawSettings
	if err := json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcModifySettings: %v", req)
	}
	//// TODO (kiashok): Test and optimize!
	// Test with crictl/ctr update resources call for wcow hyperv
	var modifySettingsRequest hcsschema.ModifySettingRequest
	var modifySettingsReqRawSettings json.RawMessage

	modifySettingsRequest.Settings = &modifySettingsReqRawSettings
	//modifySettingsRequest.GuestRequest = rawGuestRequest
	if err := json.Unmarshal(requestRawSettings, &modifySettingsRequest); err != nil {
		log.Printf("invalid rpcModifySettings request %v", r)
		return fmt.Errorf("invalid rpcModifySettings request %v", r)
	}
	log.Printf("rpcModifySettings: ModifySettingRequest %v\n", modifySettingsRequest)
	if modifySettingsRequest.ResourcePath != "" {
		reqType := modifySettingsRequest.RequestType
		resourcePath := modifySettingsRequest.ResourcePath

		log.Printf("rpcModifySettings: ModifySettingRequest { RequestType: %v \n, ResourcePath: %v", reqType, resourcePath)

		switch resourcePath {
		case resourcepaths.SiloMappedDirectoryResourcePath:
			mappedDirectory := &hcsschema.MappedDirectory{}
			if err := json.Unmarshal(modifySettingsReqRawSettings, &mappedDirectory); err != nil {
				log.Printf("invalid SiloMappedDirectoryResourcePath request %v", r)
				return fmt.Errorf("invalid SiloMappedDirectoryResourcePath request %v", r)
			}

			// TODO: check for Settings to be nil as in some examples
			log.Printf(", mappedDirectory: %v \n", mappedDirectory)
		case resourcepaths.SiloMemoryResourcePath:
			var memoryLimit uint64
			if err := json.Unmarshal(modifySettingsReqRawSettings, &memoryLimit); err != nil {
				log.Printf("invalid SiloMemoryResourcePath request %v", r)
				return fmt.Errorf("invalid SiloMemoryResourcePath request %v", r)
			}

			log.Printf(", memoryLimit: %v \n", memoryLimit)
		case resourcepaths.SiloProcessorResourcePath:
			processor := &hcsschema.Processor{}
			if err := json.Unmarshal(modifySettingsReqRawSettings, &processor); err != nil {
				log.Printf("invalid SiloProcessorResourcePath request %v", r)
				return fmt.Errorf("invalid SiloProcessorResourcePath request %v", r)
			}

			log.Printf(", processor: %v \n", processor)
		case resourcepaths.CPUGroupResourcePath:
			cpuGroup := &hcsschema.CpuGroup{}
			if err := json.Unmarshal(modifySettingsReqRawSettings, &cpuGroup); err != nil {
				log.Printf("invalid CpuGroup request %v", r)
				return fmt.Errorf("invalid CpuGroup request %v", r)
			}

			log.Printf(", cpuGroup: %v \n", cpuGroup)
		case resourcepaths.CPULimitsResourcePath:
			processorLimits := &hcsschema.ProcessorLimits{}
			if err := json.Unmarshal(modifySettingsReqRawSettings, &processorLimits); err != nil {
				log.Printf("invalid CPULimitsResourcePath request %v", r)
				return fmt.Errorf("invalid CPULimitsResourcePath request %v", r)
			}

			log.Printf(", processorLimits: %v \n", processorLimits)
		case resourcepaths.MemoryResourcePath:
			var actualMemory uint64
			if err := json.Unmarshal(modifySettingsReqRawSettings, &actualMemory); err != nil {
				log.Printf("invalid MemoryResourcePath request %v", r)
				return fmt.Errorf("invalid MemoryResourcePath request %v", r)
			}

			log.Printf(", actualMemory: %v \n", actualMemory)
		case resourcepaths.VSMBShareResourcePath:
			virtualSmbShareSettings := &hcsschema.VirtualSmbShare{}
			if err := json.Unmarshal(modifySettingsReqRawSettings, &virtualSmbShareSettings); err != nil {
				log.Printf("invalid VSMBShareResourcePath request %v", r)
				return fmt.Errorf("invalid VSMBShareResourcePath request %v", r)
			}

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
			if !isSpecialResourcePaths(resourcePath, modifySettingsReqRawSettings) {
				return fmt.Errorf("invalid rpcModifySettings resourcePath %v", resourcePath)
			}
		}
	}

	////
	var modifyGuestSettingsRequest guestrequest.ModificationRequest
	var rawGuestRequest json.RawMessage
	modifyGuestSettingsRequest.Settings = &rawGuestRequest
	if err := json.Unmarshal(requestRawSettings, &modifyGuestSettingsRequest); err != nil {
		log.Printf("invalid rpcModifySettings ModificationRequest request %v", r)
		return fmt.Errorf("invalid rpcModifySettings ModificationRequest request %v", r)
	}
	log.Printf("rpcModifySettings: ModificationRequest %v\n", modifyGuestSettingsRequest)

	//if rawGuestRequest != nil {
	guestResourceType := modifyGuestSettingsRequest.ResourceType
	guestRequestType := modifyGuestSettingsRequest.RequestType

	log.Printf("rpcModifySettings: guestRequest.ModificationRequest { resourceType: %v \n, requestType: %v", guestResourceType, guestRequestType)

	switch guestResourceType {
	case guestresource.ResourceTypeCombinedLayers:
		settings := &guestresource.WCOWCombinedLayers{}
		if err := json.Unmarshal(rawGuestRequest, settings); err != nil {
			log.Printf("invalid ResourceTypeCombinedLayers request %v", r)
			return fmt.Errorf("invalid ResourceTypeCombinedLayers request %v", r)
		}

		log.Printf(", WCOWCombinedLayers {ContainerRootPath: %v, Layers: %v, ScratchPath: %v} \n", settings.ContainerRootPath, settings.Layers, settings.ScratchPath)
	case guestresource.ResourceTypeNetworkNamespace:
		settings := &hcn.HostComputeNamespace{}
		if err := json.Unmarshal(rawGuestRequest, settings); err != nil {
			log.Printf("invalid ResourceTypeNetworkNamespace request %v", r)
			return fmt.Errorf("invalid ResourceTypeNetworkNamespace request %v", r)
		}

		log.Printf(", HostComputeNamespaces { %v} \n", settings)
	case guestresource.ResourceTypeNetwork:
		// following valid only for osversion.Build() >= osversion.RS5
		// since Cwcow is available only for latest versions this is ok
		settings := &guestrequest.NetworkModifyRequest{}
		if err := json.Unmarshal(rawGuestRequest, settings); err != nil {
			log.Printf("invalid ResourceTypeNetwork request %v", r)
			return fmt.Errorf("invalid ResourceTypeNetwork request %v", r)
		}

		log.Printf(", NetworkModifyRequest { %v} \n", settings)
	case guestresource.ResourceTypeMappedVirtualDisk:
		wcowMappedVirtualDisk := &guestresource.WCOWMappedVirtualDisk{}
		if err := json.Unmarshal(rawGuestRequest, wcowMappedVirtualDisk); err != nil {
			log.Printf("invalid ResourceTypeMappedVirtualDisk request %v", r)
			return fmt.Errorf("invalid ResourceTypeMappedVirtualDisk request %v", r)
		}
		var ctx context.Context
		err := b.PolicyEnforcer.securityPolicyEnforcer.EnforceDeviceMountPolicy(ctx, wcowMappedVirtualDisk.ContainerPath, "0123456789")
		if err != nil {
			log.Printf("denied by policy %v", r)
		}
		log.Printf(", wcowMappedVirtualDisk { %v} \n", wcowMappedVirtualDisk)
	// TODO need a case similar to guestresource.ResourceTypeSecurityPolicy of lcow?
	// case guestresource.ResourceTypeSecurityPolicy:
	case guestresource.ResourceTypeHvSocket:
		hvSocketAddress := &hcsschema.HvSocketAddress{}
		if err := json.Unmarshal(rawGuestRequest, hvSocketAddress); err != nil {
			log.Printf("invalid ResourceTypeHvSocket request %v", r)
			return fmt.Errorf("invalid ResourceTypeHvSocket request %v", r)
		}

		log.Printf(", hvSocketAddress { %v} \n", hvSocketAddress)
	case guestresource.ResourceTypeSecurityPolicy:
		securityPolicyRequest := &guestresource.WCOWConfidentialOptions{}
		if err := json.Unmarshal(rawGuestRequest, securityPolicyRequest); err != nil {
			log.Printf("invalid ResourceTypeSecurityPolicy request %v", r)
			return fmt.Errorf("invalid ResourceTypeSecurityPolicy request %v", r)
		}

		log.Printf(", WCOWConfidentialOptions: { %v} \n", securityPolicyRequest)
		_ = b.PolicyEnforcer.SetWCOWConfidentialUVMOptions( /*ctx, */ securityPolicyRequest)
		// skipSendToGCS = true
		// send response back to shim
		log.Printf("\n early response to hcsshim? \n")
		err := b.sendReplyToShim(rpcModifySettings, *req)
		if err != nil {
			//
			log.Printf("error sending early reply back to hcsshim")
			err = fmt.Errorf("error sending early reply back to hcsshim")
			return err
		}
		return nil
		//return err, skipSendToGCS
	default:
		isSpecialGuestRequests(string(guestResourceType), rawGuestRequest)
		// invalid
	}
	//}

	// If we are here, there is no error and we want to
	// forward the message to inbox GCS
	b.sendToGCSChan <- *req

	return nil
	//, skipSendToGCS
}

// TODO: cleanup helper
func (b *Bridge) sendReplyToShim(rpcProcType rpcProc, req request) error {
	respType := msgTypeResponse | msgType(rpcProcType)
	var msgBase requestBase
	_ = json.Unmarshal(req.message, msgBase)
	resp := &responseBase{
		Result: 0, // 0 means succes!
		//	ErrorMessage: "",
		//fmt.Sprintf("Request %v not allowed", req.typ.String()),
		ActivityID: msgBase.ActivityID,
	}
	msgb, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	b.sendMessageToShim(respType, getMessageID(req.header), msgb)

	return nil
}

// TODO (kiashok): Cleanup.
// Sends early reply to shim
func (b *Bridge) sendMessageToShim(typ msgType, id int64, msg []byte) {
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

func (b *Bridge) modifySettings(req *request) error {
	var err error

	log.Printf("\n rpcModifySettings handler \n")

	//skipSendToGCS := false
	if err = b.unMarshalAndModifySettings(req); err != nil {
		return err
	}

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	//	if !skipSendToGCS {
	//		b.forwardMessageToGCS(*req)
	//	}

	return nil
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

func (b *Bridge) negotiateProtocol(req *request) error {
	var r negotiateProtocolRequest
	var err error
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcNegotiateProtocol: %v", req)
	}
	log.Printf("rpcNegotiateProtocol: negotiateProtocolRequest{ requestBase %v, MinVersion: %v, MaxVersion: %v }", r.requestBase, r.MinimumVersion, r.MaximumVersion)

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) dumpStacks(req *request) error {
	var r dumpStacksRequest
	var err error
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcStart: %v", req)
	}
	log.Printf("rpcDumpStacks: \n requestBase: %v", r.requestBase)

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) deleteContainerState(req *request) error {
	var r deleteContainerStateRequest
	var err error
	if err = json.Unmarshal(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal rpcStart: %v", req)
	}
	log.Printf("rpcDeleteContainerRequest: \n requestBase: %v", r.requestBase)

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) updateContainer(req *request) error {
	// No callers in the code for rpcUpdateContainer

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return nil
}

func (b *Bridge) lifecycleNotification(req *request) error {
	// No callers in the code for rpcLifecycleNotification

	// If we've reached here, means the policy has allowed it.
	// So forward msg to inbox GCS.
	b.sendToGCSChan <- *req

	return nil
}
