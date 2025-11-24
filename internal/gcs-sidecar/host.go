//go:build windows
// +build windows

package bridge

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	oci "github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/pspdriver"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	// state required for the security policy enforcement
	policyMutex               sync.Mutex
	securityPolicyEnforcer    securitypolicy.SecurityPolicyEnforcer
	securityPolicyEnforcerSet bool
	uvmReferenceInfo          string
}

type Container struct {
	id              string
	spec            specs.Spec
	processesMutex  sync.Mutex
	processes       map[uint32]*containerProcess
	commandLine     bool
	commandLineExec bool
}

// Process is a struct that defines the lifetime and operations associated with
// an oci.Process.
type containerProcess struct {
	processspec hcsschema.ProcessParameters
	// cid is the container id that owns this process.
	cid string
	pid uint32
}

func NewHost(initialEnforcer securitypolicy.SecurityPolicyEnforcer) *Host {
	return &Host{
		containers:                make(map[string]*Container),
		securityPolicyEnforcer:    initialEnforcer,
		securityPolicyEnforcerSet: false,
	}
}

// Write security policy, signed UVM reference and host AMD certificate to
// container's rootfs, so that application and sidecar containers can have
// access to it. The security policy is required by containers which need to
// extract init-time claims found in the security policy. The directory path
// containing the files is exposed via UVM_SECURITY_CONTEXT_DIR env var.
// It may be an error to have a security policy but not expose it to the
// container as in that case it can never be checked as correct by a verifier.
func (h *Host) SetupSecurityContextDir(ctx context.Context, spec *specs.Spec) error {
	if oci.ParseAnnotationsBool(ctx, spec.Annotations, annotations.WCOWSecurityPolicyEnv, true) {
		encodedPolicy := h.securityPolicyEnforcer.EncodedSecurityPolicy()
		hostAMDCert := spec.Annotations[annotations.WCOWHostAMDCertificate]
		if len(encodedPolicy) > 0 || len(hostAMDCert) > 0 || len(h.uvmReferenceInfo) > 0 {
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
			if len(h.uvmReferenceInfo) > 0 {
				if err := writeFileInDir(securityContextDir, securitypolicy.ReferenceInfoFilename, []byte(h.uvmReferenceInfo), 0777); err != nil {
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
	issuer, feed, payloadString, err := securitypolicy.ExtractAndVerifyFragment(ctx, fragment)
	if err != nil {
		return err
	}
	// now offer the payload fragment to the policy
	err = h.securityPolicyEnforcer.LoadFragment(ctx, issuer, feed, payloadString)
	if err != nil {
		return fmt.Errorf("error loading security policy fragment: %w", err)
	}
	return nil
}

func (h *Host) SetWCOWConfidentialUVMOptions(ctx context.Context, securityPolicyRequest *guestresource.WCOWConfidentialOptions, logWriter io.Writer) error {
	h.policyMutex.Lock()
	defer h.policyMutex.Unlock()

	if h.securityPolicyEnforcerSet {
		return errors.New("security policy has already been set")
	}

	if err := pspdriver.GetPspDriverError(); err != nil {
		// For this case gcs-sidecar will keep initial deny policy.
		return errors.Wrapf(err, "an error occurred while using PSP driver")
	}

	// Fetch report and validate host_data
	hostData, err := securitypolicy.NewSecurityPolicyDigest(securityPolicyRequest.EncodedSecurityPolicy)
	if err != nil {
		return err
	}

	if err := pspdriver.ValidateHostData(ctx, hostData[:]); err != nil {
		// For this case gcs-sidecar will keep initial deny policy.
		return err
	}

	// This limit ensures messages are below the character truncation limit that
	// can be imposed by an orchestrator
	maxErrorMessageLength := 3 * 1024

	// Initialize security policy enforcer for a given enforcer type and
	// encoded security policy.
	p, err := securitypolicy.CreateSecurityPolicyEnforcer(
		securityPolicyRequest.EnforcerType,
		securityPolicyRequest.EncodedSecurityPolicy,
		DefaultCRIMounts(),
		DefaultCRIPrivilegedMounts(),
		maxErrorMessageLength,
	)
	if err != nil {
		return fmt.Errorf("error creating security policy enforcer: %w", err)
	}

	if err = p.EnforceRuntimeLoggingPolicy(ctx); err == nil {
		logrus.SetOutput(logWriter)
	} else {
		logrus.SetOutput(io.Discard)
	}

	h.securityPolicyEnforcer = p
	h.securityPolicyEnforcerSet = true
	h.uvmReferenceInfo = securityPolicyRequest.EncodedUVMReference

	return nil
}

func (h *Host) AddContainer(ctx context.Context, id string, c *Container) error {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		log.G(ctx).Tracef("Container exists in the map: %v", ok)
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}
	log.G(ctx).Tracef("AddContainer: ID: %v", id)
	h.containers[id] = c
	return nil
}

func (h *Host) RemoveContainer(ctx context.Context, id string) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	_, ok := h.containers[id]
	if !ok {
		log.G(ctx).Tracef("RemoveContainer: Container not found: ID: %v", id)
		return
	}

	delete(h.containers, id)
}

func (h *Host) GetCreatedContainer(ctx context.Context, id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	c, ok := h.containers[id]
	if !ok {
		log.G(ctx).Tracef("GetCreatedContainer: Container not found: ID: %v", id)
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	return c, nil
}

// GetProcess returns the Process with the matching 'pid'. If the 'pid' does
// not exit returns error.
func (c *Container) GetProcess(pid uint32) (*containerProcess, error) {
	//todo: thread a context to this function call
	logrus.WithFields(logrus.Fields{
		logfields.ContainerID: c.id,
		logfields.ProcessID:   pid,
	}).Info("opengcs::Container::GetProcess")

	c.processesMutex.Lock()
	defer c.processesMutex.Unlock()

	p, ok := c.processes[pid]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	return p, nil
}
