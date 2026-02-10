//go:build windows

package guestmanager

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

// SecurityPolicyManager exposes guest security policy operations.
type SecurityPolicyManager interface {
	// AddSecurityPolicy adds a security policy to the guest.
	AddSecurityPolicy(ctx context.Context, settings guestresource.ConfidentialOptions) error
	// InjectPolicyFragment injects a policy fragment into the guest.
	InjectPolicyFragment(ctx context.Context, settings guestresource.SecurityPolicyFragment) error
}

var _ SecurityPolicyManager = (*Guest)(nil)

// AddSecurityPolicy adds a security policy to the guest.
func (gm *Guest) AddSecurityPolicy(ctx context.Context, settings guestresource.ConfidentialOptions) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypeSecurityPolicy,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to add security policy: %w", err)
	}
	return nil
}

// InjectPolicyFragment injects a policy fragment into the guest.
func (gm *Guest) InjectPolicyFragment(ctx context.Context, settings guestresource.SecurityPolicyFragment) error {
	request := &hcsschema.ModifySettingRequest{
		GuestRequest: guestrequest.ModificationRequest{
			ResourceType: guestresource.ResourceTypePolicyFragment,
			RequestType:  guestrequest.RequestTypeAdd,
			Settings:     settings,
		},
	}

	err := gm.modify(ctx, request.GuestRequest)
	if err != nil {
		return fmt.Errorf("failed to inject security policy fragment: %w", err)
	}
	return nil
}
