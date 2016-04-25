package hcsshim

import (
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
)

type waitable interface {
	waitTimeoutInternal(timeout uint32) (bool, error)
	hcsWait(exitEvent *syscall.Handle, result **uint16) error
	title() string
}

func waitTimeoutHelper(object waitable, timeout time.Duration) (bool, error) {
	var (
		millis uint32
		result bool
		err    error
	)

	for totalMillis := uint64(timeout / time.Millisecond); totalMillis > 0; totalMillis = totalMillis - uint64(millis) {
		if totalMillis >= syscall.INFINITE {
			millis = syscall.INFINITE - 1
		} else {
			millis = uint32(totalMillis)
		}

		result, err = object.waitTimeoutInternal(millis)

		if err != nil || result == false {
			return result, err
		}
	}
	return result, err
}

func waitTimeoutInternalHelper(object waitable, timeout uint32) (bool, error) {
	title := "HCSShim::" + object.title() + "::waitTimeoutInternal"
	logrus.Debugf(title+" timeout=%d", timeout)
	var (
		resultp   *uint16
		exitEvent syscall.Handle
	)

	err := object.hcsWait(&exitEvent, &resultp)
	err = processHcsResult(err, resultp)
	if err != nil {
		err = makeError(err, title, "")
		logrus.Error(err)
		return false, err
	}

	s, e := syscall.WaitForSingleObject(syscall.Handle(exitEvent), timeout)
	switch s {
	case syscall.WAIT_OBJECT_0:
		logrus.Debugf(title+" succeeded timeout=%d", timeout)
		return true, nil
	case syscall.WAIT_TIMEOUT:
		return false, nil
	default:
		return false, makeError(e, title, "WaitForSingleObject failed")
	}

}
