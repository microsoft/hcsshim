package uvm

import (
	"context"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/logfields"
	"github.com/Microsoft/hcsshim/internal/requesttype"
	hcsschema "github.com/Microsoft/hcsshim/internal/schema2"
	"github.com/sirupsen/logrus"
)

// Modify modifies the compute system by sending a request to HCS.
func (uvm *UtilityVM) Modify(doc *hcsschema.ModifySettingRequest) (err error) {
	op := "uvm::Modify"
	log := logrus.WithFields(logrus.Fields{
		logfields.UVMID: uvm.id,
	})
	log.Debug(op + " - Begin Operation")
	defer func() {
		if err != nil {
			log.Data[logrus.ErrorKey] = err
			log.Error(op + " - End Operation - Error")
		} else {
			log.Debug(op + " - End Operation - Success")
		}
	}()

	if doc.GuestRequest == nil || uvm.gc == nil {
		return uvm.hcsSystem.Modify(doc)
	}

	hostdoc := *doc
	hostdoc.GuestRequest = nil
	if doc.ResourcePath != "" && doc.RequestType == requesttype.Add {
		err = uvm.hcsSystem.Modify(&hostdoc)
		if err != nil {
			return fmt.Errorf("adding VM resources: %s", err)
		}
		defer func() {
			if err != nil {
				hostdoc.RequestType = requesttype.Remove
				rerr := uvm.hcsSystem.Modify(&hostdoc)
				if rerr != nil {
					logrus.WithError(err).Error("failed to roll back resource add")
				}
			}
		}()
	}
	err = uvm.gc.Modify(context.TODO(), doc.GuestRequest)
	if err != nil {
		return fmt.Errorf("guest modify: %s", err)
	}
	if doc.ResourcePath != "" && doc.RequestType == requesttype.Remove {
		err = uvm.hcsSystem.Modify(&hostdoc)
		if err != nil {
			err = fmt.Errorf("removing VM resources: %s", err)
			logrus.WithError(err).Error("failed to remove host resources after successful guest request")
			return err
		}
	}
	return nil
}
