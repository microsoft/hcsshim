package winobjdir

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/winapi"
)

const bufferSize = 1024

// EnumerateNTObjectDirectory queries for all entires in the object
// directory at `ntObjDirPath`. returns the resulting entry names as a string slice.
func EnumerateNTObjectDirectory(ntObjDirPath string) ([]string, error) {
	var (
		handle uintptr
		oa     winapi.ObjectAttributes

		context      uint32
		returnLength uint32
		buffer       [bufferSize]byte
		result       []string
	)

	path := filepath.Clean(ntObjDirPath)
	fsNtPath := filepath.FromSlash(path)

	pathUnicode, err := winapi.NewUnicodeString(fsNtPath)
	if err != nil {
		return nil, err
	}

	oa.Length = unsafe.Sizeof(oa)
	oa.ObjectName = pathUnicode

	// open `ntObjDirPath` directory
	status := winapi.NtOpenDirectoryObject(
		&handle,
		winapi.FILE_LIST_DIRECTORY,
		&oa,
	)

	if !winapi.NTSuccess(status) {
		return nil, winapi.RtlNtStatusToDosError(status)
	}

	defer syscall.Close(syscall.Handle(handle))

	for {
		// Query opened `globalNTPath` for entries. This call takes in a
		// set length buffer, so to ensure we find all entires, we make
		// successive calls until no more entires exist or an error is seen.
		status = winapi.NtQueryDirectoryObject(
			handle,
			&buffer[0],
			bufferSize,
			false,
			false,
			&context,
			&returnLength,
		)

		if !winapi.NTSuccess(status) && status != winapi.STATUS_MORE_ENTRIES {
			if status == winapi.STATUS_NO_MORE_ENTRIES {
				break
			}
			return nil, winapi.RtlNtStatusToDosError(status)
		}
		dirInfo := (*winapi.ObjectDirectoryInformation)(unsafe.Pointer(&buffer[0]))
		index := 1
		for {
			if dirInfo == nil || dirInfo.Name.Length == 0 {
				break
			}
			result = append(result, dirInfo.Name.String())
			size := unsafe.Sizeof(winapi.ObjectDirectoryInformation{}) * uintptr(index)
			dirInfo = (*winapi.ObjectDirectoryInformation)(unsafe.Pointer(&buffer[size]))
			index++
		}
	}

	return result, nil
}
