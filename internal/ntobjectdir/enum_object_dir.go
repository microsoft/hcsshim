package ntobjectdir

import (
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/kernel32"
	"github.com/Microsoft/hcsshim/internal/ntconstants"
)

//go:generate go run ..\..\mksyscall_windows.go -output zsyscall_windows.go enum_object_dir.go

//sys ntOpenDirectoryObject(handle *uintptr, accessMask uint32, oa *ntconstants.ObjectAttributes) (status uint32) = ntdll.NtOpenDirectoryObject
//sys ntQueryDirectoryObject(handle uintptr, buffer *byte, length uint32, singleEntry bool, restartScan bool, context *uint32, returnLength *uint32)(status uint32) = ntdll.NtQueryDirectoryObject

const globalNTPath = "\\Global??"
const bufferSize = 1024

// ntWidePath converts a golang string to a wide string
func ntWidePath(path string) ([]uint16, error) {
	path = filepath.Clean(path)
	fspath := filepath.FromSlash(path)
	path16 := utf16.Encode(([]rune)(fspath))
	if len(path16) > 32767 {
		return nil, syscall.ENAMETOOLONG
	}
	return path16, nil
}

// EnumerateNTGlobalObjectDirectory queries `globalNTPath` for all entires and
// returns the resulting entry names as a string slice.
func EnumerateNTGlobalObjectDirectory() ([]string, error) {
	var (
		handle uintptr
		oa     ntconstants.ObjectAttributes

		context      uint32
		returnLength uint32
		buffer       [bufferSize]byte
		result       []string
	)

	globalNTPath16, err := ntWidePath(globalNTPath)
	if err != nil {
		return nil, err
	}

	// convert `globalNTPath16` to a unicodeString for passing to windows
	// functions.
	upathBuffer := kernel32.LocalAlloc(0, int(unsafe.Sizeof(ntconstants.UnicodeString{}))+len(globalNTPath16)*2)
	defer kernel32.LocalFree(upathBuffer)

	upath := (*ntconstants.UnicodeString)(unsafe.Pointer(upathBuffer))
	upath.Length = uint16(len(globalNTPath16) * 2)
	upath.MaximumLength = upath.Length
	upath.Buffer = upathBuffer + unsafe.Sizeof(*upath)
	copy((*[32768]uint16)(unsafe.Pointer(upath.Buffer))[:], globalNTPath16)

	oa.Length = unsafe.Sizeof(oa)
	oa.ObjectName = upathBuffer

	// open `globalNTPath` directory
	status := ntOpenDirectoryObject(
		&handle,
		ntconstants.FILE_LIST_DIRECTORY,
		&oa,
	)

	if !ntconstants.NTSuccess(status) {
		return nil, ntconstants.RtlNtStatusToDosError(status)
	}

	defer syscall.Close(syscall.Handle(handle))

	for {
		// Query opened `globalNTPath` for entries. This call takes in a
		// set length buffer, so to ensure we find all entires, we make
		// successive calls until no more entires exist or an error is seen.
		status = ntQueryDirectoryObject(
			handle,
			&buffer[0],
			bufferSize,
			false,
			false,
			&context,
			&returnLength,
		)

		if !ntconstants.NTSuccess(status) && status != ntconstants.STATUS_MORE_ENTRIES {
			if status == ntconstants.STATUS_NO_MORE_ENTRIES || status == ntconstants.ERROR_NO_MORE_ITEMS {
				break
			}
			return nil, ntconstants.RtlNtStatusToDosError(status)
		}
		dirInfo := (*ntconstants.ObjectDirectoryInformation)(unsafe.Pointer(&buffer[0]))
		index := 1
		for {
			if dirInfo == nil || dirInfo.Name.Length == 0 {
				break
			}
			result = append(result, ntconstants.ConvertToString(dirInfo.Name))
			size := unsafe.Sizeof(ntconstants.ObjectDirectoryInformation{}) * uintptr(index)
			dirInfo = (*ntconstants.ObjectDirectoryInformation)(unsafe.Pointer(&buffer[size]))
			index++
		}
	}

	return result, nil
}
