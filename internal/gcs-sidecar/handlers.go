//go:build windows
// +build windows

package bridge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/commonutils"
	"github.com/Microsoft/hcsshim/internal/fsformatter"
	"github.com/Microsoft/hcsshim/internal/gcs/prot"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	oci "github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils/etw"
	"github.com/Microsoft/hcsshim/internal/windevice"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/pkg/errors"
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

	// containerConfig can be of type uvmConfig or guestresource.CWCOWHostedSystem
	var (
		uvmConfig               prot.UvmConfig
		cwcowHostedSystemConfig guestresource.CWCOWHostedSystem
	)
	if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &uvmConfig); err == nil &&
		uvmConfig.SystemType != "" {
		systemType := uvmConfig.SystemType
		timeZoneInformation := uvmConfig.TimeZoneInformation
		log.G(ctx).Tracef("createContainer: uvmConfig: {systemType: %v, timeZoneInformation: %v}}", systemType, timeZoneInformation)
	} else if err = commonutils.UnmarshalJSONWithHresult(containerConfig, &cwcowHostedSystemConfig); err == nil &&
		cwcowHostedSystemConfig.Spec.Version != "" && cwcowHostedSystemConfig.CWCOWHostedSystem.Container != nil {
		cwcowHostedSystem := cwcowHostedSystemConfig.CWCOWHostedSystem
		schemaVersion := cwcowHostedSystem.SchemaVersion
		container := cwcowHostedSystem.Container
		spec := cwcowHostedSystemConfig.Spec
		containerID := createContainerRequest.ContainerID
		log.G(ctx).Tracef("rpcCreate: CWCOWHostedSystemConfig {spec: %v, schemaVersion: %v, container: %v}}", string(req.message), schemaVersion, container)

		// Enforce registry changes policy
		if container != nil && container.RegistryChanges != nil {
			log.G(ctx).Trace("Container has registry changes, validating against policy")

			// First, separate default values from non-default values
			var defaultValues []hcsschema.RegistryValue
			var nonDefaultValues []hcsschema.RegistryValue

			if container.RegistryChanges.AddValues != nil {
				for _, value := range container.RegistryChanges.AddValues {
					if isDefaultRegistryValue(value) {
						defaultValues = append(defaultValues, value)
						log.G(ctx).WithField("name", value.Name).Trace("Registry value matches default, accepting without policy check")
					} else {
						nonDefaultValues = append(nonDefaultValues, value)
					}
				}
			}

			// If there are non-default values, validate them against policy
			if len(nonDefaultValues) > 0 {
				log.G(ctx).Tracef("Validating %d registry values against policy", len(nonDefaultValues))

				nonDefaultChanges := &hcsschema.RegistryChanges{
					AddValues: nonDefaultValues,
				}

				err := b.hostState.securityOptions.PolicyEnforcer.EnforceRegistryChangesPolicy(ctx, containerID, nonDefaultChanges)
				if err != nil {
					log.G(ctx).WithError(err).Warn("Registry changes validation failed - rejecting")
					return fmt.Errorf("registry entry operation is denied by policy: %w", err)
				}
				log.G(ctx).Tracef("All container registry values validated successfully")
			}

			log.G(ctx).Infof("Registry validation complete: %d total values (%d defaults + %d validated)",
				len(container.RegistryChanges.AddValues), len(defaultValues), len(nonDefaultValues))
		}

		user := securitypolicy.IDName{
			Name: spec.Process.User.Username,
		}
		_, _, _, err := b.hostState.securityOptions.PolicyEnforcer.EnforceCreateContainerPolicyV2(req.ctx, containerID, spec.Process.Args, spec.Process.Env, spec.Process.Cwd, spec.Mounts, user, nil)

		if err != nil {
			return fmt.Errorf("CreateContainer operation is denied by policy: %w", err)
		}

		commandLine := len(spec.Process.Args) > 0
		c := &Container{
			id:              containerID,
			spec:            spec,
			processes:       make(map[uint32]*containerProcess),
			commandLine:     commandLine,
			commandLineExec: false,
		}

		log.G(ctx).Tracef("Adding ContainerID: %v", containerID)
		if err := b.hostState.AddContainer(req.ctx, containerID, c); err != nil {
			log.G(ctx).Tracef("Container exists in the map.")
			return err
		}
		defer func() {
			if err != nil {
				if removeErr := b.hostState.RemoveContainer(ctx, containerID); removeErr != nil {
					log.G(ctx).WithError(removeErr).Errorf("Failed to remove container: %v", containerID)
				}
			}
		}()

		if oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.WCOWSecurityPolicyEnv, true) {
			if err := b.hostState.securityOptions.WriteSecurityContextDir(&spec); err != nil {
				return fmt.Errorf("failed to write security context dir: %w", err)
			}
			cwcowHostedSystemConfig.Spec = spec
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

	err = b.hostState.securityOptions.PolicyEnforcer.EnforceShutdownContainerPolicy(req.ctx, r.ContainerID)
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
		_, _, err := b.hostState.securityOptions.PolicyEnforcer.EnforceExecExternalProcessPolicy(
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

		c.processesMutex.Lock()
		isCreateExec := c.commandLine && !c.commandLineExec
		if isCreateExec {
			// if this is an exec of Container command line, then it's already enforced
			// during container creation, hence skip it here
			c.commandLineExec = true

		}
		c.processesMutex.Unlock()
		if !isCreateExec {
			user := securitypolicy.IDName{
				Name: processParams.User,
			}
			log.G(req.ctx).Tracef("Enforcing policy on exec in container")
			_, _, _, err = b.hostState.securityOptions.PolicyEnforcer.
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

		// initiate exec process response channel
		procRespCh := make(chan *prot.ContainerExecuteProcessResponse, 1)
		b.pendingMu.Lock()
		b.pending[headerID] = procRespCh
		b.pendingMu.Unlock()

		defer func() {
			b.pendingMu.Lock()
			delete(b.pending, headerID)
			b.pendingMu.Unlock()
		}()

		// forward the request to gcs
		b.forwardRequestToGcs(req)

		// fetch the process ID from response
		select {
		case resp := <-procRespCh:
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
			// Channel closed or received nil, treat as error
			return errors.New("received nil exec response")
		case <-time.After(5 * time.Second):
			return errors.New("timed out waiting for exec response")
		}
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
		err = b.hostState.securityOptions.PolicyEnforcer.EnforceSignalContainerProcessPolicyV2(req.ctx, containerID, opts)
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

	if err := b.hostState.securityOptions.PolicyEnforcer.EnforceGetPropertiesPolicy(req.ctx); err != nil {
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
	err = b.hostState.RemoveContainer(req.ctx, r.ContainerID)
	if err != nil {
		log.G(req.ctx).Tracef("Container not found during deleteContainerState: %v", r.ContainerID)
		return fmt.Errorf("container not found: %w", err)
	}

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

func (b *Bridge) modifyServiceSettings(req *request) (err error) {
	_, span := oc.StartSpan(req.ctx, "sidecar::modifyServiceSettings")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	modifyRequest, err := unmarshalModifyServiceSettings(req)
	if err != nil {
		return fmt.Errorf("failed to unmarshal modifyServiceSettings request: %w", err)
	}

	switch modifyRequest.PropertyType {
	case string(prot.LogForwardService):
		if modifyRequest.Settings != nil {
			log.G(req.ctx).Tracef("modifyServiceSettings for LogForwardService with RPCModifyServiceSettings, enforcing policy for log sources")
			settings := modifyRequest.Settings.(*guestrequest.LogForwardServiceRPCRequest)

			switch settings.RPCType {
			case guestrequest.RPCModifyServiceSettings, guestrequest.RPCStartLogForwarding, guestrequest.RPCStopLogForwarding:
				log.G(req.ctx).Tracef("%v request received for LogForwardService, proceeding with policy enforcement for log sources", settings.RPCType)
				if settings.Settings != "" {
					// Decode the base64-encoded log sources config so we can
					// enforce policy on the requested provider list.
					logSources, err := etw.DecodeAndUnmarshalLogSources(settings.Settings)
					if err != nil {
						return fmt.Errorf("failed to decode log sources: %w", err)
					}

					// Validate host-supplied (Name, GUID) pairs before
					// name-based policy enforcement.
					if err := validateLogProviders(logSources.LogConfig.Sources); err != nil {
						return fmt.Errorf("log providers rejected: %w", err)
					}

					// Collect every requested provider name and ask the
					// enforcer to validate them as a batch. The enforcer's
					// behaviour depends on allow_log_provider_dropping in the
					// active policy:
					//   - false (default, fail-close): any disallowed provider
					//     causes the call to be denied.
					//   - true: disallowed providers are silently dropped and
					//     the kept subset is returned for forwarding.
					var requestedNames []string
					for _, source := range logSources.LogConfig.Sources {
						for _, provider := range source.Providers {
							requestedNames = append(requestedNames, provider.ProviderName)
						}
					}

					keptNames, err := b.hostState.securityOptions.PolicyEnforcer.EnforceLogProviderPolicy(
						req.ctx, requestedNames)
					if err != nil {
						return fmt.Errorf("log providers denied by policy: %w", err)
					}

					filtered := filterLogSourcesToAllowed(req.ctx, logSources, keptNames)

					// Apply GUID resolution (and any other inbox-GCS prep)
					// against the policy-trimmed payload and hand off to
					// inbox GCS.
					allowedLogSources, err := etw.UpdateLogSourcesFromInfo(filtered, false, true)
					if err != nil {
						return fmt.Errorf("failed to update log sources: %w", err)
					}
					settings.Settings = allowedLogSources
				}
			default:
				return fmt.Errorf("modifyServiceSettings for LogForwardService: unsupported RPCType %q", settings.RPCType)
			}
			modifyRequest.Settings = settings
			buf, err := json.Marshal(modifyRequest)
			if err != nil {
				return fmt.Errorf("failed to marshal modifyServiceSettings request: %w", err)
			}
			var newRequest request
			newRequest.ctx = req.ctx
			newRequest.header = req.header
			newRequest.header.Size = uint32(len(buf)) + prot.HdrSize
			newRequest.message = buf
			req = &newRequest
		} else {
			log.G(req.ctx).Warningf("modifyServiceSettings for LogForwardService with empty settings, skipping policy enforcement")
		}
	default:
		return fmt.Errorf("modifyServiceSettings: unsupported PropertyType %q", modifyRequest.PropertyType)
	}
	b.forwardRequestToGcs(req)
	return nil
}

// validateLogProviders validates host-supplied log providers before they
// reach the name-based policy enforcer.
//
// CWCOW policy approves provider names, but inbox GCS subscribes by GUID. If
// the host could send {Name: "allowed", GUID: "<disallowed>"} the name-based
// enforcer would approve and the disallowed GUID would still be forwarded
// (resolveGUIDsWithLookup keeps any GUID the host set). To close that bypass
// the sidecar rejects, before enforcement, any entry whose (Name, GUID) pair
// is not verifiable against the well-known ETW map:
//
//   - Name == "": rejected. Policy is name-based; a GUID-only entry has
//     nothing for the enforcer to evaluate.
//   - Name + GUID where Name is not in the well-known map: rejected. We have
//     no ground truth to compare the GUID against, so we cannot verify the
//     host's claim. Name-only is still accepted for downstream resolution to
//     stay best-effort.
//   - Name + GUID where the GUID disagrees with the well-known lookup for
//     Name: rejected.
//
// Name-only entries are passed through unchanged; the sidecar fills in the
// canonical GUID after enforcement via etw.UpdateLogSourcesFromInfo.
func validateLogProviders(sources []etw.Source) error {
	for _, src := range sources {
		for _, p := range src.Providers {
			if p.ProviderName == "" {
				return fmt.Errorf("provider with no name is not allowed (GUID %q)", p.ProviderGUID)
			}
			if p.ProviderGUID == "" {
				continue
			}
			well := etw.GetProviderGUIDFromName(p.ProviderName)
			if well == "" {
				return fmt.Errorf("provider %q: name not in well-known ETW map; cannot verify supplied GUID %q", p.ProviderName, p.ProviderGUID)
			}
			suppliedTrimmed := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(p.ProviderGUID), "{"), "}")
			supplied, err := guid.FromString(suppliedTrimmed)
			if err != nil {
				return fmt.Errorf("provider %q: invalid GUID %q: %w", p.ProviderName, p.ProviderGUID, err)
			}
			if !strings.EqualFold(supplied.String(), well) {
				return fmt.Errorf("provider %q: supplied GUID %q does not match well-known GUID %q", p.ProviderName, p.ProviderGUID, well)
			}
		}
	}
	return nil
}

func filterLogSourcesToAllowed(ctx context.Context, sources etw.LogSourcesInfo, keptNames []string) etw.LogSourcesInfo {
	keepSet := make(map[string]struct{}, len(keptNames))
	for _, name := range keptNames {
		keepSet[name] = struct{}{}
	}

	var requestedNames []string
	dropped := make([]string, 0)
	seenDropped := make(map[string]struct{})
	for i := range sources.LogConfig.Sources {
		src := &sources.LogConfig.Sources[i]
		filtered := make([]etw.EtwProvider, 0, len(src.Providers))
		for _, p := range src.Providers {
			requestedNames = append(requestedNames, p.ProviderName)
			if _, ok := keepSet[p.ProviderName]; ok {
				filtered = append(filtered, p)
				continue
			}
			if _, dup := seenDropped[p.ProviderName]; !dup {
				seenDropped[p.ProviderName] = struct{}{}
				dropped = append(dropped, p.ProviderName)
			}
		}
		src.Providers = filtered
	}

	if len(dropped) > 0 {
		log.G(ctx).WithFields(map[string]interface{}{
			"requested": requestedNames,
			"kept":      keptNames,
			"dropped":   dropped,
		}).Warn("log providers trimmed by policy (allow_log_provider_dropping)")
	}

	return sources
}

func volumeGUIDFromLayerPath(path string) (string, bool) {
	if p, ok := strings.CutPrefix(path, `\\?\Volume{`); ok {
		if q, ok := strings.CutSuffix(p, `}\Files`); ok {
			return q, true
		}
	}
	return "", false
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
			securityPolicyRequest := modifyGuestSettingsRequest.Settings.(*guestresource.ConfidentialOptions)
			log.G(ctx).Tracef("WCOWConfidentialOptions: { %v}", securityPolicyRequest)
			err := b.hostState.securityOptions.SetConfidentialOptions(ctx,
				securityPolicyRequest.EnforcerType,
				securityPolicyRequest.EncodedSecurityPolicy,
				securityPolicyRequest.EncodedUVMReference,
				securityPolicyRequest.EncodedUVMHashEnvelopeReference)
			if err != nil {
				return errors.Wrap(err, "Failed to set Confidentia UVM Options")
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
			r, ok := modifyGuestSettingsRequest.Settings.(*guestresource.SecurityPolicyFragment)
			if !ok {
				return errors.New("the request settings are not of type SecurityPolicyFragment")
			}
			if err := b.hostState.securityOptions.InjectFragment(ctx, r); err != nil {
				return err
			}
			resp := &prot.ResponseBase{
				Result:     0,
				ActivityID: req.activityID,
			}
			return b.sendResponseToShim(req.ctx, prot.RPCModifySettings, req.header.ID, resp)

		case guestresource.ResourceTypeWCOWBlockCims:
			// This is request to mount the merged cim at given volumeGUID
			switch modifyGuestSettingsRequest.RequestType {
			case guestrequest.RequestTypeAdd:
				wcowBlockCimMounts := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWBlockCIMMounts)
				containerID := wcowBlockCimMounts.ContainerID
				log.G(ctx).Tracef("WCOWBlockCIMMounts Add { %v}", wcowBlockCimMounts)

				var layerCIMs []*cimfs.BlockCIM
				layerHashes := make([]string, len(wcowBlockCimMounts.BlockCIMs))
				layerDigests := make([][]byte, len(wcowBlockCimMounts.BlockCIMs))
				for i, blockCimDevice := range wcowBlockCimMounts.BlockCIMs {
					// Get the scsi device path for the blockCim lun
					// The block device takes some time to show up. Retry for up to 2 seconds.
					var devNumber uint32
					waitStartTime := time.Now()
					logTime := waitStartTime.Add(time.Second)
					logged := false
					for {
						devNumber, err = windevice.GetDeviceNumberFromControllerLUN(
							req.ctx,
							0, /* controller is always 0 for wcow */
							uint8(blockCimDevice.Lun))
						if err == nil {
							break
						}

						// Check if we've exceeded max wait time
						if time.Since(waitStartTime) >= 2*time.Second {
							return fmt.Errorf("err getting scsiDevPath after 2s: %w", err)
						}

						// Log if taking longer than expected
						if !logged && logTime.Before(time.Now()) {
							log.G(ctx).WithFields(map[string]interface{}{
								"lun":     blockCimDevice.Lun,
								"elapsed": time.Since(waitStartTime),
							}).Warn("waiting for block CIM device to show up")
							logged = true
						}

						time.Sleep(10 * time.Millisecond)
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
					layerHashes[i] = hex.EncodeToString(cimRootDigestBytes)
					layerCIMs = append(layerCIMs, &layerCim)

					log.G(ctx).Debugf("block CIM layer digest %s, path: %s\n", layerHashes[i], physicalDevPath)
				}

				// Top layer is the merged layer that will also be verified
				hashesToVerify := layerHashes
				mountedCim := []string{layerHashes[0]}
				if len(layerHashes) > 1 {
					hashesToVerify = layerHashes[1:]
				}

				err := b.hostState.securityOptions.PolicyEnforcer.EnforceVerifiedCIMsPolicy(req.ctx, containerID, hashesToVerify, mountedCim)
				if err != nil {
					return errors.Wrap(err, "CIM mount is denied by policy")
				}

				// Volume GUID from request
				volGUID := wcowBlockCimMounts.VolumeGUID

				// Cache hashes along with volGUID
				b.hostState.blockCIMVolumeHashes[volGUID] = layerHashes

				// Store the containerID (associated with volGUID) to mark that hashes are verified for this container
				if _, ok := b.hostState.blockCIMVolumeContainers[volGUID]; !ok {
					b.hostState.blockCIMVolumeContainers[volGUID] = make(map[string]struct{})
				}
				b.hostState.blockCIMVolumeContainers[volGUID][containerID] = struct{}{}

				log.G(ctx).Tracef("Cached %d verified CIM layer hashes for volume %s (container %s)", len(hashesToVerify), volGUID, containerID)

				if len(layerCIMs) > 1 {
					_, err = cimfs.MountMergedVerifiedBlockCIMs(layerCIMs[0], layerCIMs[1:], wcowBlockCimMounts.MountFlags, wcowBlockCimMounts.VolumeGUID, layerDigests[0])
					if err != nil {
						return fmt.Errorf("error mounting multilayer block cims: %w", err)
					}
				} else {
					_, err = cimfs.MountVerifiedBlockCIM(layerCIMs[0], wcowBlockCimMounts.MountFlags, wcowBlockCimMounts.VolumeGUID, layerDigests[0])
					if err != nil {
						return fmt.Errorf("error mounting verified block cim: %w", err)
					}
				}

			case guestrequest.RequestTypeRemove:
				log.G(ctx).Tracef("WCOWBlockCIMMounts: Remove")
				wcowBlockCimMounts := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWBlockCIMMounts)
				volumePath := fmt.Sprintf(cimfs.VolumePathFormat, wcowBlockCimMounts.VolumeGUID.String())
				err := cimfs.Unmount(volumePath)

				if err != nil {
					return fmt.Errorf("error unmounting block cim: %w", err)
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
		case guestresource.ResourceTypeCWCOWCombinedLayers:
			settings := modifyGuestSettingsRequest.Settings.(*guestresource.CWCOWCombinedLayers)
			switch modifyGuestSettingsRequest.RequestType {
			case guestrequest.RequestTypeAdd:
				containerID := settings.ContainerID
				log.G(ctx).Tracef("CWCOWCombinedLayers:: ContainerID: %v, ContainerRootPath: %v, Layers: %v, ScratchPath: %v",
					containerID, settings.CombinedLayers.ContainerRootPath, settings.CombinedLayers.Layers, settings.CombinedLayers.ScratchPath)

				// The layers size is only one, as this is the volume path
				if len(settings.CombinedLayers.Layers) != 1 {
					return fmt.Errorf("expected exactly one layer in CWCOWCombinedLayers, got %d", len(settings.CombinedLayers.Layers))
				}
				layerPath := settings.CombinedLayers.Layers[0].Path
				guidStr, ok := volumeGUIDFromLayerPath(layerPath)
				if !ok {
					return fmt.Errorf("invalid volumeGUID %s", containerID)
				}
				volGUID, err := guid.FromString(guidStr)
				if err != nil {
					return fmt.Errorf("failed to parse volume GUID %s: %w", guidStr, err)
				}
				hashes, haveHashes := b.hostState.blockCIMVolumeHashes[volGUID]
				if haveHashes {
					// Only do this if the ContainerID is not already seen for this volume
					containers := b.hostState.blockCIMVolumeContainers[volGUID]
					if _, seen := containers[containerID]; !seen {
						// This is a container with similar layers as an existing container, hence already mounted.
						// Call EnforceVerifiedCIMsPolicy on this new container.
						hashesToVerify := hashes
						mountedCim := []string{hashes[0]}
						if len(hashes) > 1 {
							hashesToVerify = hashes[1:]
						}
						if err := b.hostState.securityOptions.PolicyEnforcer.EnforceVerifiedCIMsPolicy(ctx, containerID, hashesToVerify, mountedCim); err != nil {
							return fmt.Errorf("CIM mount is denied by policy for this container: %w", err)
						}
						log.G(ctx).Tracef("Verified CIM hashes for reused mount volume %s (container %s)", volGUID.String(), containerID)
						containers[containerID] = struct{}{}
					}
				}

				//Since unencrypted scratch is not an option, always pass true
				if err := b.hostState.securityOptions.PolicyEnforcer.EnforceScratchMountPolicy(ctx, settings.CombinedLayers.ContainerRootPath, true); err != nil {
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

			case guestrequest.RequestTypeRemove:
				log.G(ctx).Tracef("CWCOWCombinedLayers: Remove")
				if err := b.hostState.securityOptions.PolicyEnforcer.EnforceScratchUnmountPolicy(ctx, settings.CombinedLayers.ContainerRootPath); err != nil {
					return fmt.Errorf("scratch unmounting denied by policy: %w", err)
				}
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

		default:
			// Invalid request
			return fmt.Errorf("invalid modifySettingsRequest: %v", guestResourceType)
		}
	}

	b.forwardRequestToGcs(req)
	return nil
}
