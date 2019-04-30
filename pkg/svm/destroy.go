package svm

import (
	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

func (i *instance) Destroy() error {
	for id, svmItem := range i.serviceVMs {
		if err := terminate(svmItem.serviceVM.utilityVM); err != nil {
			return err
		}
		delete(i.serviceVMs, id)
	}
	return nil
}

func terminate(u *uvm.UtilityVM) error {
	if err := u.ComputeSystem().Terminate(); err != nil {
		if hcs.IsPending(err) {
			err = u.Wait()
		}
		if err != nil {
			return err
		}
	}
	return nil
}
