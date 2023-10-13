//go:build windows

package uvm

import (
	"context"
	"fmt"

	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/log"
)

// modify modifies the compute system by sending a request to HCS or to the guest connection.
func (uvm *UtilityVM) modify(ctx context.Context, doc *hcsschema.ModifySettingRequest) (err error) {
	if !doc.ValidGuestRequest() || uvm.gc == nil {
		return uvm.hcsSystem.Modify(ctx, doc)
	}

	hostdoc := *doc
	hostdoc.GuestRequest = nil
	if doc.ResourcePath != "" && doc.RequestType != nil && *doc.RequestType == hcsschema.ModifyRequestType_ADD {
		err = uvm.hcsSystem.Modify(ctx, &hostdoc)
		if err != nil {
			return fmt.Errorf("adding VM resources: %w", err)
		}
		defer func() {
			if err != nil {
				rt := hcsschema.ModifyRequestType_REMOVE
				hostdoc.RequestType = &rt
				rerr := uvm.hcsSystem.Modify(ctx, &hostdoc)
				if rerr != nil {
					log.G(ctx).WithError(rerr).Error("failed to roll back resource add")
				}
			}
		}()
	}
	err = uvm.gc.Modify(ctx, doc.GuestRequest)
	if err != nil {
		return fmt.Errorf("guest modify: %w", err)
	}
	if doc.ResourcePath != "" && doc.RequestType != nil && *doc.RequestType == hcsschema.ModifyRequestType_REMOVE {
		err = uvm.hcsSystem.Modify(ctx, &hostdoc)
		if err != nil {
			err = fmt.Errorf("removing VM resources: %w", err)
			log.G(ctx).WithError(err).Error("failed to remove host resources after successful guest request")
			return err
		}
	}
	return nil
}
