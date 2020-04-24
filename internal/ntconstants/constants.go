package ntconstants

import (
	"syscall"
	"unsafe"
)

//go:generate go run ..\..\mksyscall_windows.go -output zsyscall_windows.go constants.go

//sys RtlNtStatusToDosError(status uint32) (winerr error) = ntdll.RtlNtStatusToDosErrorNoTeb

type IOStatusBlock struct {
	Status, Information uintptr
}

type ObjectAttributes struct {
	Length             uintptr
	RootDirectory      uintptr
	ObjectName         uintptr
	Attributes         uintptr
	SecurityDescriptor uintptr
	SecurityQoS        uintptr
}

type UnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        uintptr
}

type ObjectDirectoryInformation struct {
	Name     UnicodeString
	TypeName UnicodeString
}

type FileLinkInformation struct {
	ReplaceIfExists bool
	RootDirectory   uintptr
	FileNameLength  uint32
	FileName        [1]uint16
}

type FileDispositionInformationEx struct {
	Flags uintptr
}

const (
	FileLinkInformationClass          = 11
	FileDispositionInformationExClass = 64

	FILE_READ_ATTRIBUTES  = 0x0080
	FILE_WRITE_ATTRIBUTES = 0x0100
	DELETE                = 0x10000

	FILE_OPEN   = 1
	FILE_CREATE = 2

	FILE_LIST_DIRECTORY          = 0x00000001
	FILE_DIRECTORY_FILE          = 0x00000001
	FILE_SYNCHRONOUS_IO_NONALERT = 0x00000020
	FILE_DELETE_ON_CLOSE         = 0x00001000
	FILE_OPEN_FOR_BACKUP_INTENT  = 0x00004000
	FILE_OPEN_REPARSE_POINT      = 0x00200000

	FILE_DISPOSITION_DELETE = 0x00000001

	OBJ_DONT_REPARSE = 0x1000

	STATUS_REPARSE_POINT_ENCOUNTERED = 0xC000050B
	STATUS_MORE_ENTRIES              = 0x105
	STATUS_NO_MORE_ENTRIES           = 0x8000001a

	ERROR_NO_MORE_ITEMS = 0x103
)

func NTSuccess(status uint32) bool {
	return status == 0
}

//ConvertToString converts wide strings to golang strings
func ConvertToString(uni UnicodeString) string {
	p := (*[0xffff]uint16)(unsafe.Pointer(uni.Buffer))
	return syscall.UTF16ToString(p[:])
}
