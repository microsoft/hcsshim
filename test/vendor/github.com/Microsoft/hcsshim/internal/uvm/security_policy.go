package uvm

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

// SetSecurityPolicy tells the gcs instance in the UVM what policy to apply.
//
// This has to happen before we start mounting things or generally changing
// the state of the UVM after is has been measured at startup
func (uvm *UtilityVM) SetSecurityPolicy(ctx context.Context, policy string) error {
	if uvm.operatingSystem != "linux" {
		return errNotSupported
	}

	if policy == "" {
		openDoorPolicy := securitypolicy.NewOpenDoorPolicy()
		policyString, err := openDoorPolicy.EncodeToString()
		if err != nil {
			return err
		}
		policy = policyString
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	modification := &hcsschema.ModifySettingRequest{
		RequestType: guestrequest.RequestTypeAdd,
		Settings: securitypolicy.EncodedSecurityPolicy{
			SecurityPolicy: policy,
		},
	}

	modification.GuestRequest = guestrequest.ModificationRequest{
		ResourceType: guestresource.ResourceTypeSecurityPolicy,
		RequestType:  guestrequest.RequestTypeAdd,
		Settings: securitypolicy.EncodedSecurityPolicy{
			SecurityPolicy: policy,
		},
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return fmt.Errorf("uvm::Policy: failed to modify utility VM configuration: %s", err)
	}

	return nil
}
