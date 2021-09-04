package uvm

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/guestrequest"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

var (
	ErrBadPolicy = errors.New("your policy looks suspicious or is badly formatted")
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
		return nil
	}

	uvm.m.Lock()
	defer uvm.m.Unlock()

	modification := &hcsschema.ModifySettingRequest{
		RequestType: requesttype.Add,
		Settings: securitypolicy.EncodedSecurityPolicy{
			SecurityPolicy: policy,
		},
	}

	modification.GuestRequest = guestrequest.GuestRequest{
		ResourceType: guestrequest.ResourceTypeSecurityPolicy,
		RequestType:  requesttype.Add,
		Settings: securitypolicy.EncodedSecurityPolicy{
			SecurityPolicy: policy,
		},
	}

	if err := uvm.modify(ctx, modification); err != nil {
		return fmt.Errorf("uvm::Policy: failed to modify utility VM configuration: %s", err)
	}

	return nil
}
