//go:build windows
// +build windows

package bridge

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/Microsoft/hcsshim/internal/bridgeutils/gcserr"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/internal/pspdriver"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	oci "github.com/opencontainers/runtime-spec/specs-go"
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
	id             string
	spec           oci.Spec
	processesMutex sync.Mutex
	processes      map[uint32]*containerProcess
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
