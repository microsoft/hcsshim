//go:build windows
// +build windows

package hvsocket

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/resources"
)

const (
	addressFlagPassthru            = 0x00000001
	ioCtlHVSocketUpdateAddressInfo = 0x21c004
)

type addressInfo struct {
	systemID         guid.GUID
	virtualMachineID guid.GUID
	siloID           guid.GUID
	flags            uint32
}

type addressInfoCloser struct {
	handle windows.Handle
}

var _ resources.ResourceCloser = addressInfoCloser{}

func (aic addressInfoCloser) Release(_ context.Context) error {
	return windows.CloseHandle(aic.handle)
}

// CreateContainerAddressInfo creates an address info entry in HvSocket to redirect
// the calls to the container silo inside UVM.
func CreateContainerAddressInfo(containerID, uvmID guid.GUID) (resources.ResourceCloser, error) {
	return CreateAddressInfo(containerID, uvmID, guid.GUID{}, true)
}

// CreateAddressInfo creates an address info entry in the HvSocket provider to map a
// compute system GUID to a virtual machine ID or compartment ID.
//
// `systemID` is the compute system GUID to map.
// `vmID` is the virtual machine ID to which the system GUID maps to. Must be guid.GUID{} to specify
// that the system GUID maps to a network compartment ID on the hosting system.
// `siloID` is the silo object ID to which the system GUID maps to.
// `passthru` when vmID is not guid.GUID{}, specifies whether the systemID maps to the primary
// compartment of the virtual machine (set to `false`) or to another compartment within the
// virtual machine (set to `true`)
func CreateAddressInfo(systemID, vmID, siloID guid.GUID, passthru bool) (resources.ResourceCloser, error) {
	path := fmt.Sprintf(`\\.\HvSocketSystem\AddressInfo\{%s}`, systemID)
	u16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		u16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.CREATE_NEW,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}

	addrInfo := addressInfo{
		systemID:         systemID,
		virtualMachineID: vmID,
		siloID:           siloID,
	}

	if passthru {
		addrInfo.flags |= addressFlagPassthru
	}

	var ret uint32
	if err := windows.DeviceIoControl(
		h,
		ioCtlHVSocketUpdateAddressInfo,
		(*byte)(unsafe.Pointer(&addrInfo)),
		uint32(unsafe.Sizeof(addrInfo)),
		nil,
		0,
		&ret,
		nil,
	); err != nil {
		return nil, err
	}

	return &addressInfoCloser{h}, nil
}
