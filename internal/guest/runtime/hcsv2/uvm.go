//go:build linux
// +build linux

package hcsv2

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Microsoft/cosesign1go/pkg/cosesign1"
	didx509resolver "github.com/Microsoft/didx509go/pkg/did-x509-resolver"
	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	"github.com/Microsoft/hcsshim/internal/debug"
	"github.com/Microsoft/hcsshim/internal/guest/policy"
	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/guest/runtime"
	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/overlay"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/guest/storage/plan9"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pmem"
	"github.com/Microsoft/hcsshim/internal/guest/storage/scsi"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/verity"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/mattn/go-shellwords"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// UVMContainerID is the ContainerID that will be sent on any prot.MessageBase
// for V2 where the specific message is targeted at the UVM itself.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

// Host is the structure tracking all UVM host state including all containers
// and processes.
type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	externalProcessesMutex sync.Mutex
	externalProcesses      map[int]*externalProcess

	// Rtime is the Runtime interface used by the GCS core.
	rtime            runtime.Runtime
	vsock            transport.Transport
	devNullTransport transport.Transport

	// state required for the security policy enforcement
	policyMutex               sync.Mutex
	securityPolicyEnforcer    securitypolicy.SecurityPolicyEnforcer
	securityPolicyEnforcerSet bool
	uvmReferenceInfo          string

	// logging target
	logWriter io.Writer
	// hostMounts keeps the state of currently mounted devices and file systems,
	// which is used for GCS hardening.
	hostMounts *hostMounts
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport, initialEnforcer securitypolicy.SecurityPolicyEnforcer, logWriter io.Writer) *Host {
	return &Host{
		containers:                make(map[string]*Container),
		externalProcesses:         make(map[int]*externalProcess),
		rtime:                     rtime,
		vsock:                     vsock,
		devNullTransport:          &transport.DevNullTransport{},
		securityPolicyEnforcerSet: false,
		securityPolicyEnforcer:    initialEnforcer,
		logWriter:                 logWriter,
		hostMounts:                newHostMounts(),
	}
}

// SetConfidentialUVMOptions takes guestresource.LCOWConfidentialOptions
// to set up our internal data structures we use to store and enforce
// security policy. The options can contain security policy enforcer type,
// encoded security policy and signed UVM reference information The security
// policy and uvm reference information can be further presented to workload
// containers for validation and attestation purposes.
func (h *Host) SetConfidentialUVMOptions(ctx context.Context, r *guestresource.LCOWConfidentialOptions) error {
	h.policyMutex.Lock()
	defer h.policyMutex.Unlock()
	if h.securityPolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	// this limit ensures messages are below the character truncation limit that
	// can be imposed by an orchestrator
	maxErrorMessageLength := 3 * 1024

	// Initialize security policy enforcer for a given enforcer type and
	// encoded security policy.
	p, err := securitypolicy.CreateSecurityPolicyEnforcer(
		r.EnforcerType,
		r.EncodedSecurityPolicy,
		policy.DefaultCRIMounts(),
		policy.DefaultCRIPrivilegedMounts(),
		maxErrorMessageLength,
	)
	if err != nil {
		return err
	}

	// This is one of two points at which we might change our logging.
	// At this time, we now have a policy and can determine what the policy
	// author put as policy around runtime logging.
	// The other point is on startup where we take a flag to set the default
	// policy enforcer to use before a policy arrives. After that flag is set,
	// we use the enforcer in question to set up logging as well.
	if err = p.EnforceRuntimeLoggingPolicy(ctx); err == nil {
		logrus.SetOutput(h.logWriter)
	} else {
		logrus.SetOutput(io.Discard)
	}

	hostData, err := securitypolicy.NewSecurityPolicyDigest(r.EncodedSecurityPolicy)
	if err != nil {
		return err
	}

	if err := validateHostData(hostData[:]); err != nil {
		return err
	}

	h.securityPolicyEnforcer = p
	h.securityPolicyEnforcerSet = true
	h.uvmReferenceInfo = r.EncodedUVMReference

	return nil
}

// InjectFragment extends current security policy with additional constraints
// from the incoming fragment. Note that it is base64 encoded over the bridge/
//
// There are three checking steps:
// 1 - Unpack the cose document and check it was actually signed with the cert
// chain inside its header
// 2 - Check that the issuer field did:x509 identifier is for that cert chain
// (ie fingerprint of a non leaf cert and the subject matches the leaf cert)
// 3 - Check that this issuer/feed match the requirement of the user provided
// security policy (done in the regoby LoadFragment)
func (h *Host) InjectFragment(ctx context.Context, fragment *guestresource.LCOWSecurityPolicyFragment) (err error) {
	log.G(ctx).WithField("fragment", fmt.Sprintf("%+v", fragment)).Debug("GCS Host.InjectFragment")

	raw, err := base64.StdEncoding.DecodeString(fragment.Fragment)
	if err != nil {
		return err
	}
	blob := []byte(fragment.Fragment)
	// keep a copy of the fragment, so we can manually figure out what went wrong
	// will be removed eventually. Give it a unique name to avoid any potential
	// race conditions.
	sha := sha256.New()
	sha.Write(blob)
	timestamp := time.Now()
	fragmentPath := fmt.Sprintf("fragment-%x-%d.blob", sha.Sum(nil), timestamp.UnixMilli())
	_ = os.WriteFile(filepath.Join("/tmp", fragmentPath), blob, 0644)

	unpacked, err := cosesign1.UnpackAndValidateCOSE1CertChain(raw)
	if err != nil {
		return fmt.Errorf("InjectFragment failed COSE validation: %w", err)
	}

	payloadString := string(unpacked.Payload[:])
	issuer := unpacked.Issuer
	feed := unpacked.Feed
	chainPem := unpacked.ChainPem

	log.G(ctx).WithFields(logrus.Fields{
		"issuer":   issuer, // eg the DID:x509:blah....
		"feed":     feed,
		"cty":      unpacked.ContentType,
		"chainPem": chainPem,
	}).Debugf("unpacked COSE1 cert chain")

	log.G(ctx).WithFields(logrus.Fields{
		"payload": payloadString,
	}).Tracef("unpacked COSE1 payload")

	if len(issuer) == 0 || len(feed) == 0 { // must both be present
		return fmt.Errorf("either issuer and feed must both be provided in the COSE_Sign1 protected header")
	}

	// Resolve returns a did doc that we don't need
	// we only care if there was an error or not
	_, err = didx509resolver.Resolve(unpacked.ChainPem, issuer, true)
	if err != nil {
		log.G(ctx).Printf("Badly formed fragment - did resolver failed to match fragment did:x509 from chain with purported issuer %s, feed %s - err %s", issuer, feed, err.Error())
		return err
	}

	// now offer the payload fragment to the policy
	err = h.securityPolicyEnforcer.LoadFragment(ctx, issuer, feed, payloadString)
	if err != nil {
		return fmt.Errorf("InjectFragment failed policy load: %w", err)
	}
	log.G(ctx).Printf("passed fragment into the enforcer.")

	return nil
}

func (h *Host) SecurityPolicyEnforcer() securitypolicy.SecurityPolicyEnforcer {
	return h.securityPolicyEnforcer
}

func (h *Host) Transport() transport.Transport {
	return h.vsock
}

func (h *Host) RemoveContainer(id string) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, ok := h.containers[id]
	if !ok {
		return
	}

	// delete the network namespace for standalone and sandbox containers
	criType, isCRI := c.spec.Annotations[annotations.KubernetesContainerType]
	if !isCRI || criType == "sandbox" {
		_ = RemoveNetworkNamespace(context.Background(), id)
	}

	delete(h.containers, id)
}

func (h *Host) GetCreatedContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, ok := h.containers[id]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	if c.getStatus() != containerCreated {
		return nil, fmt.Errorf("container is not in state \"created\": %w",
			gcserr.NewHresultError(gcserr.HrVmcomputeInvalidState))
	}
	return c, nil
}

func (h *Host) AddContainer(id string, c *Container) error {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}
	h.containers[id] = c
	return nil
}

func setupSandboxMountsPath(id string) (err error) {
	mountPath := specGuest.SandboxMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandboxMounts dir in sandbox %v", id)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(mountPath)
		}
	}()

	return storage.MountRShared(mountPath)
}

func setupSandboxHugePageMountsPath(id string) error {
	mountPath := specGuest.HugePagesMountsDir(id)
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create hugepage Mounts dir in sandbox %v", id)
	}

	return storage.MountRShared(mountPath)
}

func (h *Host) CreateContainer(ctx context.Context, id string, settings *prot.VMHostedContainerSettingsV2) (_ *Container, err error) {
	criType, isCRI := settings.OCISpecification.Annotations[annotations.KubernetesContainerType]
	c := &Container{
		id:             id,
		vsock:          h.vsock,
		spec:           settings.OCISpecification,
		ociBundlePath:  settings.OCIBundlePath,
		isSandbox:      criType == "sandbox",
		exitType:       prot.NtUnexpectedExit,
		processes:      make(map[uint32]*containerProcess),
		scratchDirPath: settings.ScratchDirPath,
	}
	c.setStatus(containerCreating)

	if err := h.AddContainer(id, c); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			h.RemoveContainer(id)
		}
	}()

	// Normally we would be doing policy checking here at the start of our
	// "policy gated function". However, we can't for create container as we
	// need a properly correct sandboxID which might be changed by the code
	// below that determines the sandboxID. This is a bit of future proofing
	// as currently for our single use case, the sandboxID is the same as the
	// container id

	var namespaceID string
	// for sandbox container sandboxID is same as container id
	sandboxID := id
	if isCRI {
		switch criType {
		case "sandbox":
			// Capture namespaceID if any because setupSandboxContainerSpec clears the Windows section.
			namespaceID = specGuest.GetNetworkNamespaceID(settings.OCISpecification)
			err = setupSandboxContainerSpec(ctx, id, settings.OCISpecification)
			if err != nil {
				return nil, err
			}
			defer func() {
				if err != nil {
					_ = os.RemoveAll(settings.OCIBundlePath)
				}
			}()

			if err = setupSandboxMountsPath(id); err != nil {
				return nil, err
			}

			if err = setupSandboxHugePageMountsPath(id); err != nil {
				return nil, err
			}

			if err := policy.ExtendPolicyWithNetworkingMounts(id, h.securityPolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}
		case "container":
			sid, ok := settings.OCISpecification.Annotations[annotations.KubernetesSandboxID]
			sandboxID = sid
			if !ok || sid == "" {
				return nil, errors.Errorf("unsupported 'io.kubernetes.cri.sandbox-id': '%s'", sid)
			}
			if err := setupWorkloadContainerSpec(ctx, sid, id, settings.OCISpecification, settings.OCIBundlePath); err != nil {
				return nil, err
			}

			// Add SEV device when security policy is not empty, except when privileged annotation is
			// set to "true", in which case all UVMs devices are added.
			if len(h.securityPolicyEnforcer.EncodedSecurityPolicy()) > 0 && !oci.ParseAnnotationsBool(ctx,
				settings.OCISpecification.Annotations, annotations.LCOWPrivileged, false) {
				if err := specGuest.AddDevSev(ctx, settings.OCISpecification); err != nil {
					log.G(ctx).WithError(err).Debug("failed to add SEV device")
				}
			}

			defer func() {
				if err != nil {
					_ = os.RemoveAll(settings.OCIBundlePath)
				}
			}()
			if err := policy.ExtendPolicyWithNetworkingMounts(sandboxID, h.securityPolicyEnforcer, settings.OCISpecification); err != nil {
				return nil, err
			}
		default:
			return nil, errors.Errorf("unsupported 'io.kubernetes.cri.container-type': '%s'", criType)
		}
	} else {
		// Capture namespaceID if any because setupStandaloneContainerSpec clears the Windows section.
		namespaceID = specGuest.GetNetworkNamespaceID(settings.OCISpecification)
		if err := setupStandaloneContainerSpec(ctx, id, settings.OCISpecification); err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				_ = os.RemoveAll(settings.OCIBundlePath)
			}
		}()
		if err := policy.ExtendPolicyWithNetworkingMounts(id, h.securityPolicyEnforcer,
			settings.OCISpecification); err != nil {
			return nil, err
		}
	}

	user, groups, umask, err := h.securityPolicyEnforcer.GetUserInfo(id, settings.OCISpecification.Process)
	if err != nil {
		return nil, err
	}

	seccomp, err := securitypolicy.MeasureSeccompProfile(settings.OCISpecification.Linux.Seccomp)
	if err != nil {
		return nil, err
	}

	envToKeep, capsToKeep, allowStdio, err := h.securityPolicyEnforcer.EnforceCreateContainerPolicy(
		ctx,
		sandboxID,
		id,
		settings.OCISpecification.Process.Args,
		settings.OCISpecification.Process.Env,
		settings.OCISpecification.Process.Cwd,
		settings.OCISpecification.Mounts,
		isPrivilegedContainerCreationRequest(ctx, settings.OCISpecification),
		settings.OCISpecification.Process.NoNewPrivileges,
		user,
		groups,
		umask,
		settings.OCISpecification.Process.Capabilities,
		seccomp,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "container creation denied due to policy")
	}

	if !allowStdio {
		// stdio access isn't allow for this container. Switch to the /dev/null
		// transport that will eat all input/ouput.
		c.vsock = h.devNullTransport
	}

	if envToKeep != nil {
		settings.OCISpecification.Process.Env = []string(envToKeep)
	}

	if capsToKeep != nil {
		settings.OCISpecification.Process.Capabilities = capsToKeep
	}

	// Write security policy, signed UVM reference and host AMD certificate to
	// container's rootfs, so that application and sidecar containers can have
	// access to it. The security policy is required by containers which need to
	// extract init-time claims found in the security policy. The directory path
	// containing the files is exposed via UVM_SECURITY_CONTEXT_DIR env var.
	// It may be an error to have a security policy but not expose it to the
	// container as in that case it can never be checked as correct by a verifier.
	if oci.ParseAnnotationsBool(ctx, settings.OCISpecification.Annotations, annotations.UVMSecurityPolicyEnv, true) {
		encodedPolicy := h.securityPolicyEnforcer.EncodedSecurityPolicy()
		hostAMDCert := settings.OCISpecification.Annotations[annotations.HostAMDCertificate]
		if len(encodedPolicy) > 0 || len(hostAMDCert) > 0 || len(h.uvmReferenceInfo) > 0 {
			// Use os.MkdirTemp to make sure that the directory is unique.
			securityContextDir, err := os.MkdirTemp(settings.OCISpecification.Root.Path, securitypolicy.SecurityContextDirTemplate)
			if err != nil {
				return nil, fmt.Errorf("failed to create security context directory: %w", err)
			}
			// Make sure that files inside directory are readable
			if err := os.Chmod(securityContextDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to chmod security context directory: %w", err)
			}

			if len(encodedPolicy) > 0 {
				if err := writeFileInDir(securityContextDir, securitypolicy.PolicyFilename, []byte(encodedPolicy), 0744); err != nil {
					return nil, fmt.Errorf("failed to write security policy: %w", err)
				}
			}
			if len(h.uvmReferenceInfo) > 0 {
				if err := writeFileInDir(securityContextDir, securitypolicy.ReferenceInfoFilename, []byte(h.uvmReferenceInfo), 0744); err != nil {
					return nil, fmt.Errorf("failed to write UVM reference info: %w", err)
				}
			}

			if len(hostAMDCert) > 0 {
				if err := writeFileInDir(securityContextDir, securitypolicy.HostAMDCertFilename, []byte(hostAMDCert), 0744); err != nil {
					return nil, fmt.Errorf("failed to write host AMD certificate: %w", err)
				}
			}

			containerCtxDir := fmt.Sprintf("/%s", filepath.Base(securityContextDir))
			secCtxEnv := fmt.Sprintf("UVM_SECURITY_CONTEXT_DIR=%s", containerCtxDir)
			settings.OCISpecification.Process.Env = append(settings.OCISpecification.Process.Env, secCtxEnv)
		}
	}

	// Create the BundlePath
	if err := os.MkdirAll(settings.OCIBundlePath, 0700); err != nil {
		return nil, errors.Wrapf(err, "failed to create OCIBundlePath: '%s'", settings.OCIBundlePath)
	}
	configFile := path.Join(settings.OCIBundlePath, "config.json")
	f, err := os.Create(configFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config.json at: '%s'", configFile)
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(settings.OCISpecification); err != nil {
		return nil, errors.Wrapf(err, "failed to write OCISpecification to config.json at: '%s'", configFile)
	}
	if err := writer.Flush(); err != nil {
		return nil, errors.Wrapf(err, "failed to flush writer for config.json at: '%s'", configFile)
	}

	con, err := h.rtime.CreateContainer(id, settings.OCIBundlePath, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create container")
	}
	init, err := con.GetInitProcess()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get container init process")
	}

	c.container = con
	c.initProcess = newProcess(c, settings.OCISpecification.Process, init, uint32(c.container.Pid()), true)

	// Sandbox or standalone, move the networks to the container namespace
	if criType == "sandbox" || !isCRI {
		ns, err := getNetworkNamespace(namespaceID)
		if isCRI && err != nil {
			return nil, err
		}
		// standalone is not required to have a networking namespace setup
		if ns != nil {
			if err := ns.AssignContainerPid(ctx, c.container.Pid()); err != nil {
				return nil, err
			}
			if err := ns.Sync(ctx); err != nil {
				return nil, err
			}
		}
	}

	c.setStatus(containerCreated)
	return c, nil
}

func (h *Host) modifyHostSettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) (retErr error) {
	switch req.ResourceType {
	case guestresource.ResourceTypeSCSIDevice:
		return modifySCSIDevice(ctx, req.RequestType, req.Settings.(*guestresource.SCSIDevice))
	case guestresource.ResourceTypeMappedVirtualDisk:
		mvd := req.Settings.(*guestresource.LCOWMappedVirtualDisk)
		// find the actual controller number on the bus and update the incoming request.
		var cNum uint8
		cNum, err := scsi.ActualControllerNumber(ctx, mvd.Controller)
		if err != nil {
			return err
		}
		mvd.Controller = cNum
		// first we try to update the internal state for read-write attachments.
		if !mvd.ReadOnly {
			localCtx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()
			source, err := scsi.GetDevicePath(localCtx, mvd.Controller, mvd.Lun, mvd.Partition)
			if err != nil {
				return err
			}
			if req.RequestType == guestrequest.RequestTypeAdd {
				if err := h.hostMounts.AddRWDevice(mvd.MountPath, source, mvd.Encrypted); err != nil {
					return err
				}
				defer func() {
					if retErr != nil {
						_ = h.hostMounts.RemoveRWDevice(mvd.MountPath, source)
					}
				}()
			} else if req.RequestType == guestrequest.RequestTypeRemove {
				if err := h.hostMounts.RemoveRWDevice(mvd.MountPath, source); err != nil {
					return err
				}
				defer func() {
					if retErr != nil {
						_ = h.hostMounts.AddRWDevice(mvd.MountPath, source, mvd.Encrypted)
					}
				}()
			}
		}
		return modifyMappedVirtualDisk(ctx, req.RequestType, mvd, h.securityPolicyEnforcer)
	case guestresource.ResourceTypeMappedDirectory:
		return modifyMappedDirectory(ctx, h.vsock, req.RequestType, req.Settings.(*guestresource.LCOWMappedDirectory), h.securityPolicyEnforcer)
	case guestresource.ResourceTypeVPMemDevice:
		return modifyMappedVPMemDevice(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVPMemDevice), h.securityPolicyEnforcer)
	case guestresource.ResourceTypeCombinedLayers:
		cl := req.Settings.(*guestresource.LCOWCombinedLayers)
		// when cl.ScratchPath == "", we mount overlay as read-only, in which case
		// we don't really care about scratch encryption, since the host already
		// knows about the layers and the overlayfs.
		encryptedScratch := cl.ScratchPath != "" && h.hostMounts.IsEncrypted(cl.ScratchPath)
		return modifyCombinedLayers(ctx, req.RequestType, req.Settings.(*guestresource.LCOWCombinedLayers), encryptedScratch, h.securityPolicyEnforcer)
	case guestresource.ResourceTypeNetwork:
		return modifyNetwork(ctx, req.RequestType, req.Settings.(*guestresource.LCOWNetworkAdapter))
	case guestresource.ResourceTypeVPCIDevice:
		return modifyMappedVPCIDevice(ctx, req.RequestType, req.Settings.(*guestresource.LCOWMappedVPCIDevice))
	case guestresource.ResourceTypeContainerConstraints:
		c, err := h.GetCreatedContainer(containerID)
		if err != nil {
			return err
		}
		return c.modifyContainerConstraints(ctx, req.RequestType, req.Settings.(*guestresource.LCOWContainerConstraints))
	case guestresource.ResourceTypeSecurityPolicy:
		r, ok := req.Settings.(*guestresource.LCOWConfidentialOptions)
		if !ok {
			return errors.New("the request's settings are not of type LCOWConfidentialOptions")
		}
		return h.SetConfidentialUVMOptions(ctx, r)
	case guestresource.ResourceTypePolicyFragment:
		r, ok := req.Settings.(*guestresource.LCOWSecurityPolicyFragment)
		if !ok {
			return errors.New("the request settings are not of type LCOWSecurityPolicyFragment")
		}
		return h.InjectFragment(ctx, r)
	default:
		return errors.Errorf("the ResourceType %q is not supported for UVM", req.ResourceType)
	}
}

func (h *Host) modifyContainerSettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) error {
	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}

	switch req.ResourceType {
	case guestresource.ResourceTypeContainerConstraints:
		return c.modifyContainerConstraints(ctx, req.RequestType, req.Settings.(*guestresource.LCOWContainerConstraints))
	default:
		return errors.Errorf("the ResourceType \"%s\" is not supported for containers", req.ResourceType)
	}
}

func (h *Host) ModifySettings(ctx context.Context, containerID string, req *guestrequest.ModificationRequest) error {
	if containerID == UVMContainerID {
		return h.modifyHostSettings(ctx, containerID, req)
	}
	return h.modifyContainerSettings(ctx, containerID, req)
}

// Shutdown terminates this UVM. This is a destructive call and will destroy all
// state that has not been cleaned before calling this function.
func (*Host) Shutdown() {
	_ = syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

// Called to shutdown a container
func (h *Host) ShutdownContainer(ctx context.Context, containerID string, graceful bool) error {
	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}

	err = h.securityPolicyEnforcer.EnforceShutdownContainerPolicy(ctx, containerID)
	if err != nil {
		return err
	}

	signal := unix.SIGTERM
	if !graceful {
		signal = unix.SIGKILL
	}

	return c.Kill(ctx, signal)
}

func (h *Host) SignalContainerProcess(ctx context.Context, containerID string, processID uint32, signal syscall.Signal) error {
	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return err
	}

	p, err := c.GetProcess(processID)
	if err != nil {
		return err
	}

	signalingInitProcess := (processID == c.initProcess.pid)

	// Don't allow signalProcessV2 to route around container shutdown policy
	// Sending SIGTERM or SIGKILL to a containers init process will shut down
	// the container.
	if signalingInitProcess {
		if (signal == unix.SIGTERM) || (signal == unix.SIGKILL) {
			graceful := (signal == unix.SIGTERM)
			return h.ShutdownContainer(ctx, containerID, graceful)
		}
	}

	startupArgList := p.(*containerProcess).spec.Args
	err = h.securityPolicyEnforcer.EnforceSignalContainerProcessPolicy(ctx, containerID, signal, signalingInitProcess, startupArgList)
	if err != nil {
		return err
	}

	return p.Kill(ctx, signal)
}

func (h *Host) ExecProcess(ctx context.Context, containerID string, params prot.ProcessParameters, conSettings stdio.ConnectionSettings) (_ int, err error) {
	var pid int
	var c *Container

	if params.IsExternal || containerID == UVMContainerID {
		var envToKeep securitypolicy.EnvList
		var allowStdioAccess bool
		envToKeep, allowStdioAccess, err = h.securityPolicyEnforcer.EnforceExecExternalProcessPolicy(
			ctx,
			params.CommandArgs,
			processParamEnvToOCIEnv(params.Environment),
			params.WorkingDirectory,
		)
		if err != nil {
			return pid, errors.Wrapf(err, "exec is denied due to policy")
		}

		// It makes no sense to allow access if stdio access is denied and the
		// process requires a terminal.
		if params.EmulateConsole && !allowStdioAccess {
			return pid, errors.New("exec of process that requires terminal access denied due to policy not allowing stdio access")
		}

		if envToKeep != nil {
			params.Environment = processOCIEnvToParam(envToKeep)
		}

		var tport = h.vsock
		if !allowStdioAccess {
			tport = h.devNullTransport
		}
		pid, err = h.runExternalProcess(ctx, params, conSettings, tport)
	} else if c, err = h.GetCreatedContainer(containerID); err == nil {
		// We found a V2 container. Treat this as a V2 process.
		if params.OCIProcess == nil {
			// We've already done policy enforcement for creating a container so
			// there's no policy enforcement to do for starting
			pid, err = c.Start(ctx, conSettings)
		} else {
			// Windows uses a different field for command, there's no enforcement
			// around this yet for Windows so this is Linux specific at the moment.

			var envToKeep securitypolicy.EnvList
			var capsToKeep *specs.LinuxCapabilities
			var user securitypolicy.IDName
			var groups []securitypolicy.IDName
			var umask string
			var allowStdioAccess bool

			user, groups, umask, err = h.securityPolicyEnforcer.GetUserInfo(containerID, params.OCIProcess)
			if err != nil {
				return 0, err
			}

			envToKeep, capsToKeep, allowStdioAccess, err = h.securityPolicyEnforcer.EnforceExecInContainerPolicy(
				ctx,
				containerID,
				params.OCIProcess.Args,
				params.OCIProcess.Env,
				params.OCIProcess.Cwd,
				params.OCIProcess.NoNewPrivileges,
				user,
				groups,
				umask,
				params.OCIProcess.Capabilities,
			)
			if err != nil {
				return pid, errors.Wrapf(err, "exec in container denied due to policy")
			}

			// It makes no sense to allow access if stdio access is denied and the
			// process requires a terminal.
			if params.OCIProcess.Terminal && !allowStdioAccess {
				return pid, errors.New("exec in container of process that requires terminal access denied due to policy not allowing stdio access")
			}

			if envToKeep != nil {
				params.OCIProcess.Env = envToKeep
			}

			if capsToKeep != nil {
				params.OCIProcess.Capabilities = capsToKeep
			}

			pid, err = c.ExecProcess(ctx, params.OCIProcess, conSettings)
		}
	}

	return pid, err
}

func (h *Host) GetExternalProcess(pid int) (Process, error) {
	h.externalProcessesMutex.Lock()
	defer h.externalProcessesMutex.Unlock()

	p, ok := h.externalProcesses[pid]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	return p, nil
}

func (h *Host) GetProperties(ctx context.Context, containerID string, query prot.PropertyQuery) (*prot.PropertiesV2, error) {
	err := h.securityPolicyEnforcer.EnforceGetPropertiesPolicy(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "get properties denied due to policy")
	}

	c, err := h.GetCreatedContainer(containerID)
	if err != nil {
		return nil, err
	}

	properties := &prot.PropertiesV2{}
	for _, requestedProperty := range query.PropertyTypes {
		if requestedProperty == prot.PtProcessList {
			pids, err := c.GetAllProcessPids(ctx)
			if err != nil {
				return nil, err
			}
			properties.ProcessList = make([]prot.ProcessDetails, len(pids))
			for i, pid := range pids {
				if specGuest.OutOfUint32Bounds(pid) {
					return nil, errors.Errorf("PID (%d) exceeds uint32 bounds", pid)
				}
				properties.ProcessList[i].ProcessID = uint32(pid)
			}
		} else if requestedProperty == prot.PtStatistics {
			cgroupMetrics, err := c.GetStats(ctx)
			if err != nil {
				return nil, err
			}
			properties.Metrics = cgroupMetrics
		}
	}

	return properties, nil
}

func (h *Host) GetStacks(ctx context.Context) (string, error) {
	err := h.securityPolicyEnforcer.EnforceDumpStacksPolicy(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "dump stacks denied due to policy")
	}

	return debug.DumpStacks(), nil
}

// RunExternalProcess runs a process in the utility VM.
func (h *Host) runExternalProcess(
	ctx context.Context,
	params prot.ProcessParameters,
	conSettings stdio.ConnectionSettings,
	tport transport.Transport,
) (_ int, err error) {
	var stdioSet *stdio.ConnectionSet
	stdioSet, err = stdio.Connect(tport, conSettings)
	if err != nil {
		return -1, err
	}
	defer func() {
		if err != nil {
			stdioSet.Close()
		}
	}()

	args := params.CommandArgs
	if len(args) == 0 {
		args, err = processParamCommandLineToOCIArgs(params.CommandLine)
		if err != nil {
			return -1, err
		}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = params.WorkingDirectory
	cmd.Env = processParamEnvToOCIEnv(params.Environment)

	var relay *stdio.TtyRelay
	if params.EmulateConsole {
		// Allocate a console for the process.
		var (
			master      *os.File
			consolePath string
		)
		master, consolePath, err = stdio.NewConsole()
		if err != nil {
			return -1, errors.Wrap(err, "failed to create console for external process")
		}
		defer func() {
			if err != nil {
				master.Close()
			}
		}()

		var console *os.File
		console, err = os.OpenFile(consolePath, os.O_RDWR|syscall.O_NOCTTY, 0777)
		if err != nil {
			return -1, errors.Wrap(err, "failed to open console file for external process")
		}
		defer console.Close()

		relay = stdio.NewTtyRelay(stdioSet, master)
		cmd.Stdin = console
		cmd.Stdout = console
		cmd.Stderr = console
		// Make the child process a session leader and adopt the pty as
		// the controlling terminal.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
			Ctty:    syscall.Stdin,
		}
	} else {
		var fileSet *stdio.FileSet
		fileSet, err = stdioSet.Files()
		if err != nil {
			return -1, errors.Wrap(err, "failed to set cmd stdio")
		}
		defer fileSet.Close()
		defer stdioSet.Close()
		cmd.Stdin = fileSet.In
		cmd.Stdout = fileSet.Out
		cmd.Stderr = fileSet.Err
	}

	onRemove := func(pid int) {
		h.externalProcessesMutex.Lock()
		delete(h.externalProcesses, pid)
		h.externalProcessesMutex.Unlock()
	}
	p, err := newExternalProcess(ctx, cmd, relay, onRemove)
	if err != nil {
		return -1, err
	}

	h.externalProcessesMutex.Lock()
	h.externalProcesses[p.Pid()] = p
	h.externalProcessesMutex.Unlock()
	return p.Pid(), nil
}

func newInvalidRequestTypeError(rt guestrequest.RequestType) error {
	return errors.Errorf("the RequestType %q is not supported", rt)
}

func modifySCSIDevice(
	ctx context.Context,
	rt guestrequest.RequestType,
	msd *guestresource.SCSIDevice,
) error {
	switch rt {
	case guestrequest.RequestTypeRemove:
		cNum, err := scsi.ActualControllerNumber(ctx, msd.Controller)
		if err != nil {
			return err
		}
		return scsi.UnplugDevice(ctx, cNum, msd.Lun)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVirtualDisk(
	ctx context.Context,
	rt guestrequest.RequestType,
	mvd *guestresource.LCOWMappedVirtualDisk,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	var verityInfo *guestresource.DeviceVerityInfo
	if mvd.ReadOnly {
		// The only time the policy is empty, and we want it to be empty
		// is when no policy is provided, and we default to open door
		// policy. In any other case, e.g. explicit open door or any
		// other rego policy we would like to mount layers with verity.
		if len(securityPolicy.EncodedSecurityPolicy()) > 0 {
			devPath, err := scsi.GetDevicePath(ctx, mvd.Controller, mvd.Lun, mvd.Partition)
			if err != nil {
				return err
			}
			verityInfo, err = verity.ReadVeritySuperBlock(ctx, devPath)
			if err != nil {
				return err
			}
		}
	}
	switch rt {
	case guestrequest.RequestTypeAdd:
		mountCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if mvd.MountPath != "" {
			if mvd.ReadOnly {
				var deviceHash string
				if verityInfo != nil {
					deviceHash = verityInfo.RootDigest
				}
				err = securityPolicy.EnforceDeviceMountPolicy(ctx, mvd.MountPath, deviceHash)
				if err != nil {
					return errors.Wrapf(err, "mounting scsi device controller %d lun %d onto %s denied by policy", mvd.Controller, mvd.Lun, mvd.MountPath)
				}
			}
			config := &scsi.Config{
				Encrypted:        mvd.Encrypted,
				VerityInfo:       verityInfo,
				EnsureFilesystem: mvd.EnsureFilesystem,
				Filesystem:       mvd.Filesystem,
				BlockDev:         mvd.BlockDev,
			}
			return scsi.Mount(mountCtx, mvd.Controller, mvd.Lun, mvd.Partition, mvd.MountPath,
				mvd.ReadOnly, mvd.Options, config)
		}
		return nil
	case guestrequest.RequestTypeRemove:
		if mvd.MountPath != "" {
			if mvd.ReadOnly {
				if err := securityPolicy.EnforceDeviceUnmountPolicy(ctx, mvd.MountPath); err != nil {
					return fmt.Errorf("unmounting scsi device at %s denied by policy: %w", mvd.MountPath, err)
				}
			}
			config := &scsi.Config{
				Encrypted:        mvd.Encrypted,
				VerityInfo:       verityInfo,
				EnsureFilesystem: mvd.EnsureFilesystem,
				Filesystem:       mvd.Filesystem,
				BlockDev:         mvd.BlockDev,
			}
			if err := scsi.Unmount(ctx, mvd.Controller, mvd.Lun, mvd.Partition,
				mvd.MountPath, config); err != nil {
				return err
			}
		}
		return nil
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedDirectory(
	ctx context.Context,
	vsock transport.Transport,
	rt guestrequest.RequestType,
	md *guestresource.LCOWMappedDirectory,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		err = securityPolicy.EnforcePlan9MountPolicy(ctx, md.MountPath)
		if err != nil {
			return errors.Wrapf(err, "mounting plan9 device at %s denied by policy", md.MountPath)
		}

		return plan9.Mount(ctx, vsock, md.MountPath, md.ShareName, uint32(md.Port), md.ReadOnly)
	case guestrequest.RequestTypeRemove:
		err = securityPolicy.EnforcePlan9UnmountPolicy(ctx, md.MountPath)
		if err != nil {
			return errors.Wrapf(err, "unmounting plan9 device at %s denied by policy", md.MountPath)
		}

		return storage.UnmountPath(ctx, md.MountPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPMemDevice(ctx context.Context,
	rt guestrequest.RequestType,
	vpd *guestresource.LCOWMappedVPMemDevice,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	var verityInfo *guestresource.DeviceVerityInfo
	var deviceHash string
	if len(securityPolicy.EncodedSecurityPolicy()) > 0 {
		if vpd.MappingInfo != nil {
			return fmt.Errorf("multi mapping is not supported with verity")
		}
		verityInfo, err = verity.ReadVeritySuperBlock(ctx, pmem.GetDevicePath(vpd.DeviceNumber))
		if err != nil {
			return err
		}
		deviceHash = verityInfo.RootDigest
	}
	switch rt {
	case guestrequest.RequestTypeAdd:
		err = securityPolicy.EnforceDeviceMountPolicy(ctx, vpd.MountPath, deviceHash)
		if err != nil {
			return errors.Wrapf(err, "mounting pmem device %d onto %s denied by policy", vpd.DeviceNumber, vpd.MountPath)
		}

		return pmem.Mount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, verityInfo)
	case guestrequest.RequestTypeRemove:
		if err := securityPolicy.EnforceDeviceUnmountPolicy(ctx, vpd.MountPath); err != nil {
			return errors.Wrapf(err, "unmounting pmem device from %s denied by policy", vpd.MountPath)
		}

		return pmem.Unmount(ctx, vpd.DeviceNumber, vpd.MountPath, vpd.MappingInfo, verityInfo)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyMappedVPCIDevice(ctx context.Context, rt guestrequest.RequestType, vpciDev *guestresource.LCOWMappedVPCIDevice) error {
	switch rt {
	case guestrequest.RequestTypeAdd:
		return pci.WaitForPCIDeviceFromVMBusGUID(ctx, vpciDev.VMBusGUID)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyCombinedLayers(
	ctx context.Context,
	rt guestrequest.RequestType,
	cl *guestresource.LCOWCombinedLayers,
	scratchEncrypted bool,
	securityPolicy securitypolicy.SecurityPolicyEnforcer,
) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		layerPaths := make([]string, len(cl.Layers))
		for i, layer := range cl.Layers {
			layerPaths[i] = layer.Path
		}

		var upperdirPath string
		var workdirPath string
		readonly := false
		if cl.ScratchPath == "" {
			// The user did not pass a scratch path. Mount overlay as readonly.
			readonly = true
		} else {
			upperdirPath = filepath.Join(cl.ScratchPath, "upper")
			workdirPath = filepath.Join(cl.ScratchPath, "work")

			if err := securityPolicy.EnforceScratchMountPolicy(ctx, cl.ScratchPath, scratchEncrypted); err != nil {
				return fmt.Errorf("scratch mounting denied by policy: %w", err)
			}
		}

		if err := securityPolicy.EnforceOverlayMountPolicy(ctx, cl.ContainerID, layerPaths, cl.ContainerRootPath); err != nil {
			return fmt.Errorf("overlay creation denied by policy: %w", err)
		}

		return overlay.MountLayer(ctx, layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly)
	case guestrequest.RequestTypeRemove:
		if err := securityPolicy.EnforceOverlayUnmountPolicy(ctx, cl.ContainerRootPath); err != nil {
			return errors.Wrap(err, "overlay removal denied by policy")
		}

		return storage.UnmountPath(ctx, cl.ContainerRootPath, true)
	default:
		return newInvalidRequestTypeError(rt)
	}
}

func modifyNetwork(ctx context.Context, rt guestrequest.RequestType, na *guestresource.LCOWNetworkAdapter) (err error) {
	switch rt {
	case guestrequest.RequestTypeAdd:
		ns := GetOrAddNetworkNamespace(na.NamespaceID)
		if err := ns.AddAdapter(ctx, na); err != nil {
			return err
		}
		// This code doesnt know if the namespace was already added to the
		// container or not so it must always call `Sync`.
		return ns.Sync(ctx)
	case guestrequest.RequestTypeRemove:
		ns := GetOrAddNetworkNamespace(na.ID)
		if err := ns.RemoveAdapter(ctx, na.ID); err != nil {
			return err
		}
		return nil
	default:
		return newInvalidRequestTypeError(rt)
	}
}

// processParamCommandLineToOCIArgs converts a CommandLine field from
// ProcessParameters (a space separate argument string) into an array of string
// arguments which can be used by an oci.Process.
func processParamCommandLineToOCIArgs(commandLine string) ([]string, error) {
	args, err := shellwords.Parse(commandLine)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse command line string \"%s\"", commandLine)
	}
	return args, nil
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

// processOCIEnvToParam is the inverse of processParamEnvToOCIEnv
func processOCIEnvToParam(envs []string) map[string]string {
	paramEnv := make(map[string]string, len(envs))
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		paramEnv[parts[0]] = parts[1]
	}

	return paramEnv
}

// isPrivilegedContainerCreationRequest returns if a given container
// creation request would create a privileged container
func isPrivilegedContainerCreationRequest(ctx context.Context, spec *specs.Spec) bool {
	return oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.LCOWPrivileged, false)
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
