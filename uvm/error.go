package uvm

import (
	"fmt"
	"syscall"

	"github.com/Microsoft/hcsshim/internal/hcs"
	"github.com/Microsoft/hcsshim/internal/hcserror"
)

type HcsError = hcserror.HcsError

// UtilityVMError is an error encountered in HCS during an operation on a UtilityVM object
type UtilityVMError struct {
	UtilityVM *UtilityVM
	Operation string
	ExtraInfo string
	Err       error
	Events    []hcs.ErrorEvent
}

func convertSystemError(err error, uvm *UtilityVM) error {
	if serr, ok := err.(*hcs.SystemError); ok {
		return &UtilityVMError{UtilityVM: uvm, Operation: serr.Op, ExtraInfo: serr.Extra, Err: serr.Err, Events: serr.Events}
	}
	return err
}

func (e *UtilityVMError) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.UtilityVM == nil {
		return "unexpected nil UtilityVM for error: " + e.Err.Error()
	}

	s := "UtilityVM " + e.UtilityVM.id

	if e.Operation != "" {
		s += " encountered an error during " + e.Operation
	}

	switch e.Err.(type) {
	case nil:
		break
	case syscall.Errno:
		s += fmt.Sprintf(": failure in a Windows system call: %s (0x%x)", e.Err, hcserror.Win32FromError(e.Err))
	default:
		s += fmt.Sprintf(": %s", e.Err.Error())
	}

	for _, ev := range e.Events {
		s += "\n" + ev.String()
	}

	if e.ExtraInfo != "" {
		s += " extra info: " + e.ExtraInfo
	}

	return s
}
