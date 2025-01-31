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

func CreateAddressInfo(cid, vmid guid.GUID, passthru bool) (resources.ResourceCloser, error) {
	path := fmt.Sprintf(`\\.\HvSocketSystem\AddressInfo\{%s}`, cid)
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
		systemID:         cid,
		virtualMachineID: vmid,
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
