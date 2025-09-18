//go:build windows
// +build windows

package bridge

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/fsformatter"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/windevice"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

const (
	sandboxStateDirName = "WcSandboxState"
	hivesDirName        = "Hives"
	devPathFormat       = "\\\\.\\PHYSICALDRIVE%d"
	UVMContainerID      = "00000000-0000-0000-0000-000000000000"
)

// - Handler functions handle the incoming message requests. It
// also enforces security policy for confidential cwcow containers.
// - These handler functions may do some additional processing before
// forwarding requests to inbox GCS or send responses back to hcsshim.
// - In case of any error encountered during processing, appropriate error
// messages are returned and responses are sent back to hcsshim from ListenAndServer().
// TODO (kiashok): Verbose logging is for WIP and will be removed eventually.
func (b *Bridge) createContainer(req *request) (err error) {
	ctx, span := oc.StartSpan(req.ctx, "sidecar::createContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var createContainerRequest prot.ContainerCreate
	var containerConfig json.RawMessage
	createContainerRequest.ContainerConfig.Value = &containerConfig
	if err = commonutils.UnmarshalJSONWithHresult(req.message, &createContainerRequest); err != nil {
		return errors.Wrap(err, "failed to unmarshal createContainer")
	}

	// containerConfig can be of type uvnConfig or hcsschema.HostedSystem or guestresource.CWCOWHostedSystem
	var (
		uvmConfig               prot.UvmConfig
		hostedSystemConfig      hcsschema.HostedSystem
		cwcowHostedSystemConfig guestresource.CWCOWHostedSystem
	)
	if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &uvmConfig); err == nil &&
		uvmConfig.SystemType != "" {
		systemType := uvmConfig.SystemType
		timeZoneInformation := uvmConfig.TimeZoneInformation
		log.G(ctx).Tracef("createContainer: uvmConfig: {systemType: %v, timeZoneInformation: %v}}", systemType, timeZoneInformation)
	} else if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &hostedSystemConfig); err == nil &&
		hostedSystemConfig.SchemaVersion != nil && hostedSystemConfig.Container != nil {
		schemaVersion := hostedSystemConfig.SchemaVersion
		container := hostedSystemConfig.Container
		log.G(ctx).Tracef("rpcCreate: HostedSystemConfig: {schemaVersion: %v, container: %v}}", schemaVersion, container)
	} else if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &cwcowHostedSystemConfig); err == nil &&
		cwcowHostedSystemConfig.Spec.Version != "" && cwcowHostedSystemConfig.CWCOWHostedSystem.Container != nil {
		cwcowHostedSystem := cwcowHostedSystemConfig.CWCOWHostedSystem
		schemaVersion := cwcowHostedSystem.SchemaVersion
		container := cwcowHostedSystem.Container
		spec := cwcowHostedSystemConfig.Spec
		containerID := createContainerRequest.ContainerID
		log.G(ctx).Tracef("rpcCreate: CWCOWHostedSystemConfig {spec: %v, schemaVersion: %v, container: %v}}", string(req.message), schemaVersion, container)

		user := securitypolicy.IDName{
			Name: spec.Process.User.Username,
		}
		_, _, _, err := b.hostState.securityPolicyEnforcer.EnforceCreateContainerPolicyV2(req.ctx, containerID, spec.Process.Args, spec.Process.Env, spec.Process.Cwd, spec.Mounts, user, nil)

		if err != nil {
			return fmt.Errorf("CreateContainer operation is denied by policy: %w", err)
		}
		c := &Container{
			id:        containerID,
			spec:      spec,
			processes: make(map[uint32]*containerProcess),
		}
		log.G(ctx).Tracef("Adding ContainerID: %v", containerID)
		if err := b.hostState.AddContainer(req.ctx, containerID, c); err != nil {
			log.G(ctx).Tracef("Container exists in the map.")
			return err
		}
		defer func(err error) {
			if err != nil {
				b.hostState.RemoveContainer(ctx, containerID)
			}
		}(err)
		// Write security policy, signed UVM reference and host AMD certificate to
		// container's rootfs, so that application and sidecar containers can have
		// access to it. The security policy is required by containers which need to
		// extract init-time claims found in the security policy. The directory path
		// containing the files is exposed via UVM_SECURITY_CONTEXT_DIR env var.
		// It may be an error to have a security policy but not expose it to the
		// container as in that case it can never be checked as correct by a verifier.
		if oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.WCOWSecurityPolicyEnv, true) {
			encodedPolicy := b.hostState.securityPolicyEnforcer.EncodedSecurityPolicy()
			hostAMDCert := spec.Annotations[annotations.WCOWHostAMDCertificate]
			if len(encodedPolicy) > 0 || len(hostAMDCert) > 0 || len(b.hostState.uvmReferenceInfo) > 0 {
				// Use os.MkdirTemp to make sure that the directory is unique.
				securityContextDir, err := os.MkdirTemp(spec.Root.Path, securitypolicy.SecurityContextDirTemplate)
				if err != nil {
					return fmt.Errorf("failed to create security context directory: %w", err)
				}
				// Make sure that files inside directory are readable
				if err := os.Chmod(securityContextDir, 0755); err != nil {
					return fmt.Errorf("failed to chmod security context directory: %w", err)
				}

				if len(encodedPolicy) > 0 {
					if err := writeFileInDir(securityContextDir, securitypolicy.PolicyFilename, []byte(encodedPolicy), 0777); err != nil {
						return fmt.Errorf("failed to write security policy: %w", err)
					}
				}
				if len(b.hostState.uvmReferenceInfo) > 0 {
					if err := writeFileInDir(securityContextDir, securitypolicy.ReferenceInfoFilename, []byte(b.hostState.uvmReferenceInfo), 0777); err != nil {
						return fmt.Errorf("failed to write UVM reference info: %w", err)
					}
				}

				if len(hostAMDCert) > 0 {
					if err := writeFileInDir(securityContextDir, securitypolicy.HostAMDCertFilename, []byte(hostAMDCert), 0777); err != nil {
						return fmt.Errorf("failed to write host AMD certificate: %w", err)
					}
				}

				containerCtxDir := fmt.Sprintf("/%s", filepath.Base(securityContextDir))
				secCtxEnv := fmt.Sprintf("UVM_SECURITY_CONTEXT_DIR=%s", containerCtxDir)
				spec.Process.Env = append(spec.Process.Env, secCtxEnv)
			}
		}

		// Strip the spec field
		hostedSystemBytes, err := json.Marshal(cwcowHostedSystem)

		if err != nil {
			return fmt.Errorf("failed to marshal hostedSystem: %w", err)
		}

		// marshal it again into a JSON-escaped string which inbox GCS expects
		hostedSystemEscapedBytes, err := json.Marshal(string(hostedSystemBytes))
		if err != nil {
			return fmt.Errorf("failed to marshal hostedSystem JSON: %w", err)
		}

		// Prepare a fixed struct that takes in raw message
		type containerCreateModified struct {
			prot.RequestBase
			ContainerConfig json.RawMessage
		}
		createContainerRequestModified := containerCreateModified{
			RequestBase:     createContainerRequest.RequestBase,
			ContainerConfig: hostedSystemEscapedBytes,
		}

		buf, err := json.Marshal(createContainerRequestModified)
		log.G(ctx).Tracef("marshaled request buffer: %s", string(buf))
		if err != nil {
			return fmt.Errorf("failed to marshal rpcCreatecontainer: %w", err)
		}
		var newRequest request
		newRequest.ctx = req.ctx
		newRequest.header = req.header
		newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
		newRequest.message = buf
		req = &newRequest
	} else {
		return fmt.Errorf("invalid request to createContainer")
	}

	b.forwardRequestToGcs(req)
	return nil
}

func writeFileInDir(dir string, filename string, data []byte, perm os.FileMode) error {
	st, err := os.Stat(dir)
	if err != nil {
		return err
	}

	if !st.IsDir() {
		return fmt.Errorf("not a directory %q", dir)
	}

	targetFilename := filepath.Join(dir, filename)
	return os.WriteFile(targetFilename, data, perm)
}

// processParamEnvToOCIEnv converts an Environment field from ProcessParameters
// (a map from environment variable to value) into an array of environment
// variable assignments (where each is in the form "<variable>=<value>") which
// can be used by an oci.Process.
func processParamEnvToOCIEnv(environment map[string]string) []string {
	environmentList := make([]string, 0, len(environment))
	for k, v := range environment {
		// TODO: Do we need to escape things like quotation marks in
		// environment variable values?
		environmentList = append(environmentList, fmt.Sprintf("%s=%s", k, v))
	}
	return environmentList
}

func (b *Bridge) startContainer(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::startContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.RequestBase
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal startContainer: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) shutdownGraceful(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::shutdownGraceful")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.RequestBase
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal shutdownGraceful: %w", err)
	}

	err = b.hostState.securityPolicyEnforcer.EnforceShutdownContainerPolicy(req.ctx, r.ContainerID)
	if err != nil {
		return fmt.Errorf("rpcShudownGraceful operation not allowed: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) shutdownForced(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::shutdownForced")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.RequestBase
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal shutdownForced: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

// escapeArgs makes a Windows-style escaped command line from a set of arguments.
func escapeArgs(args []string) string {
	escapedArgs := make([]string, len(args))
	for i, a := range args {
		escapedArgs[i] = windows.EscapeArg(a)
	}
	return strings.Join(escapedArgs, " ")
}

func (b *Bridge) executeProcess(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::executeProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerExecuteProcess
	var processParamSettings json.RawMessage
	r.Settings.ProcessParameters.Value = &processParamSettings
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal executeProcess: %w", err)
	}
	containerID := r.ContainerID
	var processParams hcsschema.ProcessParameters
	if err := commonutils.UnmarshalJSONWithHresult(processParamSettings, &processParams); err != nil {
		return fmt.Errorf("executeProcess: invalid params type for request: %w", err)
	}

	commandLine := []string{processParams.CommandLine}

	if containerID == UVMContainerID {
		log.G(req.ctx).Tracef("Enforcing policy on external exec process")
		_, _, err := b.hostState.securityPolicyEnforcer.EnforceExecExternalProcessPolicy(
			req.ctx,
			commandLine,
			processParamEnvToOCIEnv(processParams.Environment),
			processParams.WorkingDirectory,
		)
		if err != nil {
			return errors.Wrapf(err, "exec is denied due to policy")
		}
		b.forwardRequestToGcs(req)
	} else {
		// fetch the container command line
		c, err := b.hostState.GetCreatedContainer(req.ctx, containerID)
		if err != nil {
			log.G(req.ctx).Tracef("Container not found during exec: %v", containerID)
			return fmt.Errorf("failed to get created container: %w", err)
		}

		// if this is an exec of Container command line, then it's already enforced
		// during container creation, hence skip it here
		containerCommandLine := escapeArgs(c.spec.Process.Args)
		if processParams.CommandLine != containerCommandLine {

			user := securitypolicy.IDName{
				Name: processParams.User,
			}

			log.G(req.ctx).Tracef("Enforcing policy on exec in container")
			_, _, _, err = b.hostState.securityPolicyEnforcer.
				EnforceExecInContainerPolicyV2(
					req.ctx,
					containerID,
					commandLine,
					processParamEnvToOCIEnv(processParams.Environment),
					processParams.WorkingDirectory,
					user,
					nil,
				)
			if err != nil {
				return errors.Wrapf(err, "exec in container denied due to policy")
			}
		}
		headerID := req.header.ID

		// initiate process ID
		b.pendingMu.Lock()
		b.pending[headerID] = nil // nil means not yet received
		b.pendingMu.Unlock()

		defer func() {
			b.pendingMu.Lock()
			delete(b.pending, headerID)
			b.pendingMu.Unlock()
		}()

		// forward the request to gcs
		b.forwardRequestToGcs(req)

		// fetch the process ID from response
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			log.G(req.ctx).Tracef("waiting for exec resp")
			b.pendingMu.Lock()
			resp := b.pending[headerID]
			b.pendingMu.Unlock()

			// capture the Process details, so that we can later enforce
			// on the allowed signals on the Process
			if resp != nil {
				log.G(req.ctx).Tracef("Got response: %+v", resp)
				c.processesMutex.Lock()
				defer c.processesMutex.Unlock()
				c.processes[resp.ProcessID] = &containerProcess{
					processspec: processParams,
					cid:         c.id,
					pid:         resp.ProcessID,
				}
				return nil
			}
			time.Sleep(10 * time.Millisecond) // backoff
		}

		return errors.Wrap(err, "timedout waiting for exec response")
	}
	return nil
}

func (b *Bridge) waitForProcess(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::waitForProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerWaitForProcess
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal waitForProcess: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) signalProcess(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::signalProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerSignalProcess
	var rawOpts json.RawMessage
	r.Options = &rawOpts
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal signalProcess: %w", err)
	}
	var wcowOptions guestresource.SignalProcessOptionsWCOW
	if rawOpts != nil {
		if err := commonutils.UnmarshalJSONWithHresult(rawOpts, &wcowOptions); err != nil {
			return fmt.Errorf("signalProcess: invalid Options type for request: %w", err)
		}

		log.G(req.ctx).Tracef("RawOpts are not nil")
		containerID := r.ContainerID
		c, err := b.hostState.GetCreatedContainer(req.ctx, containerID)
		if err != nil {
			return fmt.Errorf("failed to get created container: %w", err)
		}

		p, err := c.GetProcess(r.ProcessID)
		if err != nil {
			log.G(req.ctx).Tracef("Process not found %v", r.ProcessID)
			return err
		}
		cmdLine := p.processspec.CommandLine
		commandLine := []string{cmdLine}
		opts := &securitypolicy.SignalContainerOptions{
			IsInitProcess:  false,
			WindowsSignal:  wcowOptions.Signal,
			WindowsCommand: commandLine,
		}
		err = b.hostState.securityPolicyEnforcer.EnforceSignalContainerProcessPolicyV2(req.ctx, containerID, opts)
		if err != nil {
			return err
		}

	}
	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) resizeConsole(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::resizeConsole")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.ContainerResizeConsole
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal resizeConsole: %v", req)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) getProperties(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::getProperties")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	if err := b.hostState.securityPolicyEnforcer.EnforceGetPropertiesPolicy(req.ctx); err != nil {
		return errors.Wrapf(err, "get properties denied due to policy")
	}

	var getPropReqV2 prot.ContainerGetPropertiesV2
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &getPropReqV2); err != nil {
		return fmt.Errorf("failed to unmarshal getProperties: %v: %w", string(req.message), err)
	}
	log.G(req.ctx).Tracef("getProperties query: %v", getPropReqV2.Query.PropertyTypes)

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) negotiateProtocol(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::negotiateProtocol")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.NegotiateProtocolRequest
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal negotiateProtocol")
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) dumpStacks(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::dumpStacks")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.DumpStacksRequest
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal dumpStacks: %w", err)
	}

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) deleteContainerState(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::deleteContainerState")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	var r prot.DeleteContainerStateRequest
	if err := commonutils.UnmarshalJSONWithHresult(req.message, &r); err != nil {
		return fmt.Errorf("failed to unmarshal deleteContainerState: %w", err)
	}
	_, err = b.hostState.GetCreatedContainer(req.ctx, r.ContainerID)
	if err != nil {
		log.G(req.ctx).Tracef("Container not found during deleteContainerState: %v", r.ContainerID)
		return fmt.Errorf("container not found: %w", err)
	}
	// remove container state regardless of delete's success
	defer b.hostState.RemoveContainer(req.ctx, r.ContainerID)

	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) updateContainer(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::updateContainer")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// No callers in the code for rpcUpdateContainer
	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) lifecycleNotification(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::lifecycleNotification")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	// No callers in the code for rpcLifecycleNotification
	b.forwardRequestToGcs(req)
	return nil
}

func (b *Bridge) modifySettings(req *request) (err error) {
	ctx, span := oc.StartSpan(req.ctx, "sidecar::modifySettings")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	log.G(ctx).Tracef("modifySettings: MsgType: %v, Payload: %v", req.header.Type, string(req.message))
	modifyRequest, err := unmarshalContainerModifySettings(req)
	if err != nil {
		return err
	}
	modifyGuestSettingsRequest := modifyRequest.Request.(*guestrequest.ModificationRequest)
	guestResourceType := modifyGuestSettingsRequest.ResourceType
	guestRequestType := modifyGuestSettingsRequest.RequestType
	log.G(ctx).Tracef("modifySettings: resourceType: %v, requestType: %v", guestResourceType, guestRequestType)

	if guestRequestType == "" {
		guestRequestType = guestrequest.RequestTypeAdd
	}

	switch guestRequestType {
	case guestrequest.RequestTypeAdd:
	case guestrequest.RequestTypeRemove:
	case guestrequest.RequestTypePreAdd:
	case guestrequest.RequestTypeUpdate:
	default:
		return fmt.Errorf("invald guestRequestType %v", guestRequestType)
	}

	if guestResourceType != "" {
		switch guestResourceType {
		case guestresource.ResourceTypeCombinedLayers:
			settings := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWCombinedLayers)
			log.G(ctx).Tracef("WCOWCombinedLayers: {%v}", settings)

		case guestresource.ResourceTypeNetworkNamespace:
			settings := modifyGuestSettingsRequest.Settings.(*hcn.HostComputeNamespace)
			log.G(ctx).Tracef("HostComputeNamespaces { %v}", settings)

		case guestresource.ResourceTypeNetwork:
			settings := modifyGuestSettingsRequest.Settings.(*guestrequest.NetworkModifyRequest)
			log.G(ctx).Tracef("NetworkModifyRequest { %v}", settings)

		case guestresource.ResourceTypeMappedVirtualDisk:
			wcowMappedVirtualDisk := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWMappedVirtualDisk)
			log.G(ctx).Tracef("wcowMappedVirtualDisk { %v}", wcowMappedVirtualDisk)

		case guestresource.ResourceTypeHvSocket:
			hvSocketAddress := modifyGuestSettingsRequest.Settings.(*hcsschema.HvSocketAddress)
			log.G(ctx).Tracef("hvSocketAddress { %v }", hvSocketAddress)

		case guestresource.ResourceTypeMappedDirectory:
			settings := modifyGuestSettingsRequest.Settings.(*hcsschema.MappedDirectory)
			log.G(ctx).Tracef("hcsschema.MappedDirectory { %v }", settings)

		case guestresource.ResourceTypeSecurityPolicy:
			securityPolicyRequest := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWConfidentialOptions)
			log.G(ctx).Tracef("WCOWConfidentialOptions: { %v}", securityPolicyRequest)
			err := b.hostState.SetWCOWConfidentialUVMOptions(req.ctx, securityPolicyRequest, b.logWriter)
			if err != nil {
				return errors.Wrap(err, "error creating enforcer")
			}
			// Send response back to shim
			resp := &prot.ResponseBase{
				Result:     0, // 0 means success
				ActivityID: req.activityID,
			}
			err = b.sendResponseToShim(req.ctx, prot.RPCModifySettings, req.header.ID, resp)
			if err != nil {
				return fmt.Errorf("error sending response to hcsshim: %w", err)
			}
			return nil
		case guestresource.ResourceTypePolicyFragment:
			//Note: Reusing the same type LCOWSecurityPolicyFragment for CWCOW.
			r, ok := modifyGuestSettingsRequest.Settings.(*guestresource.LCOWSecurityPolicyFragment)
			if !ok {
				return errors.New("the request settings are not of type LCOWSecurityPolicyFragment")
			}
			return b.hostState.InjectFragment(ctx, r)
		case guestresource.ResourceTypeWCOWBlockCims:
			// This is request to mount the merged cim at given volumeGUID
			if modifyGuestSettingsRequest.RequestType == guestrequest.RequestTypeRemove {
				return fmt.Errorf("not implemented")
			}

			wcowBlockCimMounts := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWBlockCIMMounts)
			containerID := wcowBlockCimMounts.ContainerID
			log.G(ctx).Tracef("WCOWBlockCIMMounts { %v}", wcowBlockCimMounts)

			// The block device takes some time to show up. Wait for a few seconds.
			time.Sleep(2 * time.Second)

			//TODO(Mahati) : test and verify CIM hashes
			var layerCIMs []*cimfs.BlockCIM
			layerHashes := make([]string, len(wcowBlockCimMounts.BlockCIMs))
			layerDigests := make([][]byte, len(wcowBlockCimMounts.BlockCIMs))
			for i, blockCimDevice := range wcowBlockCimMounts.BlockCIMs {
				// Get the scsi device path for the blockCim lun
				devNumber, err := windevice.GetDeviceNumberFromControllerLUN(
					req.ctx,
					0, /* controller is always 0 for wcow */
					uint8(blockCimDevice.Lun))
				if err != nil {
					return fmt.Errorf("err getting scsiDevPath: %w", err)
				}
				physicalDevPath := fmt.Sprintf(devPathFormat, devNumber)
				layerCim := cimfs.BlockCIM{
					Type:      cimfs.BlockCIMTypeDevice,
					BlockPath: physicalDevPath,
					CimName:   blockCimDevice.CimName,
				}
				cimRootDigestBytes, err := cimfs.GetVerificationInfo(physicalDevPath)
				if err != nil {
					return fmt.Errorf("failed to get CIM verification info: %w", err)
				}
				layerDigests[i] = cimRootDigestBytes
				layerHashes[i] = base64.URLEncoding.EncodeToString(cimRootDigestBytes)
				layerCIMs = append(layerCIMs, &layerCim)

				log.G(ctx).Debugf("block CIM layer digest %s, path: %s\n", layerHashes[i], physicalDevPath)
			}

			// skip the merged cim and verify individual layer hashes
			hashesToVerify := layerHashes
			if len(layerHashes) > 1 {
				hashesToVerify = layerHashes[1:]
			}

			err := b.hostState.securityPolicyEnforcer.EnforceVerifiedCIMsPolicy(req.ctx, containerID, hashesToVerify)
			if err != nil {
				return errors.Wrap(err, "CIM mount is denied by policy")
			}

			if len(layerCIMs) > 1 {
				// Get the topmost merge CIM and invoke the MountMergedBlockCIMs
				_, err := cimfs.MountMergedBlockCIMs(layerCIMs[0], layerCIMs[1:], wcowBlockCimMounts.MountFlags, wcowBlockCimMounts.VolumeGUID)
				if err != nil {
					return fmt.Errorf("error mounting multilayer block cims: %w", err)
				}
			} else {
				_, err := cimfs.Mount(filepath.Join(layerCIMs[0].BlockPath, layerCIMs[0].CimName), wcowBlockCimMounts.VolumeGUID, wcowBlockCimMounts.MountFlags)
				if err != nil {
					return fmt.Errorf("error mounting merged block cims: %w", err)
				}
			}

			// Send response back to shim
			resp := &prot.ResponseBase{
				Result:     0, // 0 means success
				ActivityID: req.activityID,
			}
			err = b.sendResponseToShim(req.ctx, prot.RPCModifySettings, req.header.ID, resp)
			if err != nil {
				return fmt.Errorf("error sending response to hcsshim: %w", err)
			}
			return nil

		case guestresource.ResourceTypeCWCOWCombinedLayers:

			if modifyGuestSettingsRequest.RequestType == guestrequest.RequestTypeRemove {
				return fmt.Errorf("not implemented")
			}

			settings := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWCombinedLayers)
			containerID := settings.ContainerID
			log.G(ctx).Tracef("CWCOWCombinedLayers:: ContainerID: %v, ContainerRootPath: %v, Layers: %v, ScratchPath: %v",
				containerID, settings.CombinedLayers.ContainerRootPath, settings.CombinedLayers.Layers, settings.CombinedLayers.ScratchPath)

			//Since unencrypted scratch is not an option, always pass true
			if err := b.hostState.securityPolicyEnforcer.EnforceScratchMountPolicy(ctx, settings.CombinedLayers.ContainerRootPath, true); err != nil {
				return fmt.Errorf("scratch mounting denied by policy: %w", err)
			}
			// The following two folders are expected to be present in the scratch.
			// But since we have just formatted the scratch we would need to
			// create them manually.
			sandboxStateDirectory := filepath.Join(settings.CombinedLayers.ContainerRootPath, sandboxStateDirName)
			err = os.Mkdir(sandboxStateDirectory, 0777)
			if err != nil {
				return fmt.Errorf("failed to create sandboxStateDirectory: %w", err)
			}

			hivesDirectory := filepath.Join(settings.CombinedLayers.ContainerRootPath, hivesDirName)
			err = os.Mkdir(hivesDirectory, 0777)
			if err != nil {
				return fmt.Errorf("failed to create hivesDirectory: %w", err)
			}

			// Reconstruct WCOWCombinedLayers{} req before forwarding to GCS
			// as GCS does not understand ResourceTypeCWCOWCombinedLayers
			modifyGuestSettingsRequest.ResourceType = guestresource.ResourceTypeCombinedLayers
			modifyGuestSettingsRequest.Settings = settings.CombinedLayers
			modifyRequest.Request = modifyGuestSettingsRequest
			buf, err := json.Marshal(modifyRequest)
			if err != nil {
				return fmt.Errorf("failed to marshal rpcModifySettings: %w", err)
			}
			var newRequest request
			newRequest.ctx = req.ctx
			newRequest.header = req.header
			newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
			newRequest.message = buf
			req = &newRequest

		case guestresource.ResourceTypeMappedVirtualDiskForContainerScratch:
			wcowMappedVirtualDisk := modifyGuestSettingsRequest.Settings.(*guestresource.WCOWMappedVirtualDisk)
			log.G(ctx).Tracef("ResourceTypeMappedVirtualDiskForContainerScratch: { %v }", wcowMappedVirtualDisk)

			// This will return the volume path of the mounted scratch.
			// Scratch disk should be >= 30 GB for refs formatter to work.
			// fsFormatter understands only virtualDevObjectPathFormat. Therefore fetch the
			// disk number for the corresponding lun
			var devNumber uint32
			// It could take a few seconds for the attached scsi disk
			// to show up inside the UVM. Therefore adding retry logic
			// with delay here.
			for try := 0; try < 5; try++ {
				time.Sleep(1 * time.Second)
				devNumber, err = windevice.GetDeviceNumberFromControllerLUN(req.ctx,
					0, /* Only one controller allowed in wcow hyperv */
					uint8(wcowMappedVirtualDisk.Lun))
				if err != nil {
					if try == 4 {
						// bail out
						return fmt.Errorf("error getting diskNumber for LUN %d: %w", wcowMappedVirtualDisk.Lun, err)
					}
					continue
				} else {
					log.G(ctx).Tracef("DiskNumber of lun %d is:  %d", wcowMappedVirtualDisk.Lun, devNumber)
					break
				}
			}
			diskPath := fmt.Sprintf(fsformatter.VirtualDevObjectPathFormat, devNumber)
			log.G(ctx).Tracef("diskPath: %v, diskNumber: %v ", diskPath, devNumber)
			mountedVolumePath, err := fsformatter.InvokeFsFormatter(req.ctx, diskPath)
			if err != nil {
				return fmt.Errorf("failed to invoke refsFormatter: %w", err)
			}
			log.G(ctx).Tracef("mountedVolumePath returned from InvokeFsFormatter: %v", mountedVolumePath)

			// Forward the req as is to inbox gcs and let it retreive the volume.
			// While forwarding request to inbox gcs, make sure to replace the
			// resourceType to ResourceTypeMappedVirtualDisk that inbox GCS
			// understands.
			modifyGuestSettingsRequest.ResourceType = guestresource.ResourceTypeMappedVirtualDisk
			modifyRequest.Request = modifyGuestSettingsRequest
			buf, err := json.Marshal(modifyRequest)
			if err != nil {
				return fmt.Errorf("failed to marshal WCOWMappedVirtualDisk: %w", err)
			}
			var newRequest request
			newRequest.ctx = req.ctx
			newRequest.header = req.header
			newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
			newRequest.message = buf
			req = &newRequest

		default:
			// Invalid request
			return fmt.Errorf("invald modifySettingsRequest: %v", guestResourceType)
		}
	}

	b.forwardRequestToGcs(req)
	return nil
}
