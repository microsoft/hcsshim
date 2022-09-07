//go:build windows

package uvm

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

type ConfidentialUVMOpt func(ctx context.Context, r *guestresource.LCOWConfidentialOptions) error

func WithSecurityPolicy(policy string) ConfidentialUVMOpt {
	return func(ctx context.Context, r *guestresource.LCOWConfidentialOptions) error {
		if policy == "" {
			openDoorPolicy := securitypolicy.NewOpenDoorPolicy()
			policyString, err := openDoorPolicy.EncodeToString()
			if err != nil {
				return err
			}
			policy = policyString
		}
		r.EncodedSecurityPolicy = policy
		return nil
	}
}

func WithSecurityPolicyEnforcer(enforcer string) ConfidentialUVMOpt {
	return func(ctx context.Context, r *guestresource.LCOWConfidentialOptions) error {
		r.EnforcerType = enforcer
		return nil
	}
}

func WithSignedUVMMeasurement(measurementPath string) ConfidentialUVMOpt {
	return func(ctx context.Context, r *guestresource.LCOWConfidentialOptions) error {
		var encodedMeasurement string
		content, err := os.ReadFile(measurementPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			encodedMeasurement = base64.StdEncoding.EncodeToString(content)
		}
		r.EncodedSignedMeasurement = encodedMeasurement
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
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	confOpts := &guestresource.LCOWConfidentialOptions{}
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
		return fmt.Errorf("uvm::Policy: failed to modify utility VM configuration: %s", err)
	}

	return nil
}
