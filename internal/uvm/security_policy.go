//go:build windows

package uvm

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"
)

type ConfidentialUVMOpt func(ctx context.Context, r *guestresource.ConfidentialOptions) error

// WithSecurityPolicy sets the desired security policy for the resource.
func WithSecurityPolicy(policy string) ConfidentialUVMOpt {
	return func(ctx context.Context, r *guestresource.ConfidentialOptions) error {
		r.EncodedSecurityPolicy = policy
		return nil
	}
}

// WithSecurityPolicyEnforcer sets the desired enforcer type for the resource.
func WithSecurityPolicyEnforcer(enforcer string) ConfidentialUVMOpt {
	return func(ctx context.Context, r *guestresource.ConfidentialOptions) error {
		r.EnforcerType = enforcer
		return nil
	}
}

func base64EncodeFileContents(filePath string) (string, error) {
	if filePath == "" {
		return "", nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

// WithUVMReferenceInfo reads UVM reference info file and base64 encodes the
// content before setting it for the resource. This is no-op if the
// `referenceName` is empty or the file doesn't exist.
func WithUVMReferenceInfo(referenceRoot string, referenceName string) ConfidentialUVMOpt {
	return func(ctx context.Context, r *guestresource.ConfidentialOptions) error {
		if referenceName == "" {
			return nil
		}
		fullFilePath := filepath.Join(referenceRoot, referenceName)
		encoded, err := base64EncodeFileContents(fullFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				log.G(ctx).WithField("filePath", fullFilePath).Debug("UVM reference info file not found")
				return nil
			}
			return fmt.Errorf("failed to read UVM reference info file: %w", err)
		}
		r.EncodedUVMReference = encoded
		return nil
	}
}

// SetConfidentialUVMOptions sends information required to run the UVM on
// SNP hardware, e.g., security policy and enforcer type, signed UVM reference
// information, etc.
//
// This has to happen before we start mounting things or generally changing
// the state of the UVM after is has been measured at startup
func (uvm *UtilityVM) SetConfidentialUVMOptions(ctx context.Context, opts ...ConfidentialUVMOpt) error {
	uvm.m.Lock()
	defer uvm.m.Unlock()

	confOpts := &guestresource.ConfidentialOptions{}
	for _, o := range opts {
		if err := o(ctx, confOpts); err != nil {
			return err
		}
	}
	modification := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeAdd,
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeSecurityPolicy,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     *confOpts,
		},
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return fmt.Errorf("uvm::Policy: failed to modify utility VM configuration: %w", err)
	}

	return nil
}

// InjectPolicyFragment sends policy fragment to GCS.
func (uvm *UtilityVM) InjectPolicyFragment(ctx context.Context, fragment *ctrdtaskapi.PolicyFragment) error {

	mod := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeUpdate,
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypePolicyFragment,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings: guestresource.SecurityPolicyFragment{
				Fragment: fragment.Fragment,
			},
		},
	}
	return uvm.modify(ctx, mod)
}

// returns if this instance of the UtilityVM is created with confidential policy
func (uvm *UtilityVM) HasConfidentialPolicy() bool {
	switch opts := uvm.createOpts.(type) {
	case *OptionsWCOW:
		return opts.SecurityPolicyEnabled
	case *OptionsLCOW:
		return opts.SecurityPolicyEnabled
	default:
		panic("unexpected options type")
	}
}
