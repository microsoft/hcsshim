// This package exposes HResults as an OS-neutral struct for interop between
// Windows and Linux and defines common errors.
package errdefs

import (
	"encoding/json"
	"errors"
	"fmt"
)

// todo: add errors from internal\hcs\errors.go

const unknownErrorMessageFmt = "Unknown error (%x)"

//  HRESULTs are 32 bit values laid out as follows:
//
//   3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1
//   1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
//  +-+-+-+-+-+---------------------+-------------------------------+
//  |S|R|C|N|X|    Facility         |               Code            |
//  +-+-+-+-+-+---------------------+-------------------------------+
//
//  where
//
//      S - Severity - indicates success/fail
//          0 - Success
//          1 - Fail
//
//      R - Reserved - corresponds to NT's second severity bit. If N is
//          0, this must also be 0.
//
//      C - Customer - indicates if the value is customer defined
//          0 - Microsoft-defined
//          1 - Customer-defined
//
//      N - If set, indicates a mapped NT status value.
//
//      X -  Reserved. Should be 0.
//
//      Facility - is the facility code
//
//      Code - is the facility's status code

const (
	hresultCodeMask      = 0x0000ffff
	hresultFacilityShift = 16
	hresultFacilityMask  = 0x1fff << hresultFacilityShift
	hresultSeverityShift = 31
	hresultSeverityMask  = 0x1 << hresultSeverityShift

	FACILITY_WIN32 = 0x0007
)

// HRresult (HRESULT) is a numerical return code that indicates an operation status.
// It is not an (Win32) error code, and does not necessarily indicate failure.
//
// https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-erref/6b46e050-0761-44b1-858b-9b37a74ca32e
// https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-erref/0642cb2f-2075-4469-918c-4441e69c548a
type HResult int32

var (
	Ok = HResult(0)

	// ErrOk is the error message returned when the HResult indicates success.
	ErrOk             = errors.New("The operation completed successfully.")                                   //nolint:stylecheck //ST1005
	ErrUntranslatable = errors.New("An HRESULT could not be translated to a corresponding Win32 error code.") //nolint:stylecheck //ST1005
)

var _ json.Marshaler = &Ok
var _ json.Unmarshaler = &Ok

func NewHResult(s, f, c uint32) HResult {
	return HResult((s<<hresultSeverityShift)&hresultSeverityMask |
		(f<<hresultFacilityShift)&hresultFacilityMask |
		c&hresultCodeMask)
}

// HResultFromErrno converts a Win32 error code to an HResult.
//
// https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-erref/0c0bcf55-277e-4120-b5dc-f6115fc8dc38
func HResultFromErrno(n uint32) HResult {
	if int32(n) > 0 {
		n = (n & hresultCodeMask) | (FACILITY_WIN32 << hresultFacilityShift) | hresultSeverityMask
	}
	return HResult(int32(n))
}

func (r *HResult) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}

	return json.Marshal(r.Code())
}

func (r *HResult) UnmarshalJSON(b []byte) error {
	if r == nil {
		return fmt.Errorf("nil receiver passed to UnmarshalJSON")
	}

	var c uint32
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("invalid HResult %q: %w", b, err)
	}
	*r = HResult(c)
	return nil
}

func (r HResult) String() string {
	return r.error().Error()
}

func (r HResult) error() error {
	if m, ok := _hresultMessages[r.Errno()]; ok {
		return m
	}
	return ErrUntranslatable
}

func (r HResult) IsError() bool {
	return r < 0
}

func (r HResult) IsCustomDefined() bool {
	return r < 0
}

func (r HResult) Facility() uint32 {
	return (uint32(r) & hresultFacilityMask) >> hresultFacilityShift
}

func (r HResult) Code() uint32 {
	return uint32(r) & hresultCodeMask
}

func (r HResult) Errno() uint32 {
	if r.IsError() {
		c := uint32(r)
		if (c & hresultFacilityMask) == (FACILITY_WIN32 << hresultFacilityShift) {
			return r.Code()
		}
		return c
	}
	return 0
}

// todo: autogenerate via powershell (([System.ComponentModel.Win32Exception](<hresult>)).Message)
// or golang

// maping from Win32 error codes to their string value
// https://docs.microsoft.com/en-us/openspecs/windows_protocols/ms-erref/18d8fbe8-a967-4f1c-ae50-99ca8e491d2d
var _hresultMessages = map[uint32]error{
	0:      ErrOk,
	0x36fd: ErrUntranslatable,
}
