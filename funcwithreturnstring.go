package hcsshim

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/Sirupsen/logrus"
)

func convertParameters(parameters ...interface{}) ([]uintptr, error) {
	var args []uintptr
	for _, parameter := range parameters {
		var parameterp uintptr
		switch t := parameter.(type) {
		case string:
			temp, err1 := syscall.UTF16PtrFromString(t)
			if err1 != nil {
				return nil, fmt.Errorf("Failed conversion of parameter %s to pointer %s", t, err1)
			}
			parameterp = uintptr(unsafe.Pointer(temp))
		case int:
			parameterp = uintptr(t)
		case uintptr:
			parameterp = t
		default:
			return nil, fmt.Errorf("Unknown type encountered")
		}

		args = append(args, parameterp)
	}
	return args, nil
}

// Execute helper to run vmcompute functions
func funcWithReturnString(funcName string, parameters ...interface{}) (rstring string, err error) {
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

	// Load the OLE DLL and get a handle to the CoTaskMemFree procedure
	dll2, proc2, err := loadAndFindFromDll(oleDLLName, procCoTaskMemFree)
	if dll2 != nil {
		defer dll2.Release()
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

	var output uintptr
	args = append(args, uintptr(unsafe.Pointer(&output)))

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

	// Defer the cleanup of the memory using CoTaskMemFree
	defer proc2.Call(output)
	rstring = syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(output))[:])

	logrus.Debugf(title + "- succeeded")
	return
}
