package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/trace"
)

// Modify modifies the compute system by sending a request to HCS.
func (uvm *UtilityVM) Modify(ctx context.Context, doc *hcsschema.ModifySettingRequest) (err error) {
	ctx, span := trace.StartSpan(ctx, "uvm::Modify")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute(logfields.UVMID, uvm.id))

	if doc.GuestRequest == nil || uvm.gc == nil {
		return uvm.hcsSystem.Modify(ctx, doc)
	}

	hostdoc := *doc
	hostdoc.GuestRequest = nil
	if doc.ResourcePath != "" && doc.RequestType == requesttype.Add {
		err = uvm.hcsSystem.Modify(ctx, &hostdoc)
		if err != nil {
			return fmt.Errorf("adding VM resources: %s", err)
		}
		defer func() {
			if err != nil {
				hostdoc.RequestType = requesttype.Remove
				rerr := uvm.hcsSystem.Modify(ctx, &hostdoc)
				if rerr != nil {
					logrus.WithError(err).Error("failed to roll back resource add")
				}
			}
		}()
	}
	err = uvm.gc.Modify(ctx, doc.GuestRequest)
	if err != nil {
		return fmt.Errorf("guest modify: %s", err)
	}
	if doc.ResourcePath != "" && doc.RequestType == requesttype.Remove {
		err = uvm.hcsSystem.Modify(ctx, &hostdoc)
		if err != nil {
			err = fmt.Errorf("removing VM resources: %s", err)
			logrus.WithError(err).Error("failed to remove host resources after successful guest request")
			return err
		}
	}
	return nil
}
