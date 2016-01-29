package hcsshim

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Sirupsen/logrus"
)

// Execute helper to run vmcompute functions
func funcWithNoReturn(funcName string, parameters ...interface{}) (err error) {

	title := "HCSShim::" + funcName
	logrus.Debugln(title)

	// Load the DLL and get a handle to the procedure we need
	dll, proc, err := loadAndFind(funcName)
	if dll != nil {
		defer dll.Release()
	}
	if err != nil {
		return
	}

	var args []uintptr
	args, err = convertParameters(parameters...)
	if err != nil {
		logrus.Error(err)
		return
	}

	// Call the procedure itself.
	r1, _, _ := proc.Call(args...)

	for _, argi := range args {
		use(unsafe.Pointer(argi))
	}

	if r1 != 0 {
		err = fmt.Errorf(title+" - Win32 API call returned error r1=%d err=%s", r1, syscall.Errno(r1))
		logrus.Error(err)
		return
	}
	logrus.Debugf(title + "- succeeded")
	return
}
