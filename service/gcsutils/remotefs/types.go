package remotefs

import (
	"os"
	"time"
)

const (
	PathError          = "PathError"
	LinkError          = "LinkError"
	SyscallError       = "SyscallError"
	GenericErrorString = "GenericErrorString"
)

// ErrorString is a trivial implementation of the error interface.
type ErrorString struct {
	Err string
}

func (e *ErrorString) Error() string {
	return e.Err
}

// ExportedError is the serialized version of the a Go error.
type ExportedError struct {
	ErrType string
	*os.PathError
	*os.LinkError
	*os.SyscallError
	*ErrorString
}

// FileInfo is the stat struct returned by the remotefs system. It
// fulfills the os.FileInfo interface.
type FileInfo struct {
	NameVar    string
	SizeVar    int64
	ModeVar    os.FileMode
	ModTimeVar time.Time
	IsDirVar   bool
}

var _ os.FileInfo = &FileInfo{}

func (f *FileInfo) Name() string       { return f.NameVar }
func (f *FileInfo) Size() int64        { return f.SizeVar }
func (f *FileInfo) Mode() os.FileMode  { return f.ModeVar }
func (f *FileInfo) ModTime() time.Time { return f.ModTimeVar }
func (f *FileInfo) IsDir() bool        { return f.IsDirVar }
func (f *FileInfo) Sys() interface{}   { return nil }
