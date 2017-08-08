package remotefs

import (
	"os"
	"time"
)

// ExportedError is the serialized version of the a Go error.
// It also provides a trivial implementation of the error interface.
type ExportedError struct {
	ErrString string
	ErrNum    int `json:",omitempty"`
}

// Error returns an error string
func (ee *ExportedError) Error() string {
	return ee.ErrString
}

// FileInfo is the stat struct returned by the remotefs system. It
// fulfills the os.FileInfo interface.
type FileInfo struct {
	NameVar    string
	SizeVar    int64
	ModeVar    os.FileMode
	ModTimeVar int64 // Serialization of time.Time breaks in travis, so use an int
	IsDirVar   bool
}

var _ os.FileInfo = &FileInfo{}

// Name returns the filename from a FileInfo structure
func (f *FileInfo) Name() string { return f.NameVar }

// Size returns the size from a FileInfo structure
func (f *FileInfo) Size() int64 { return f.SizeVar }

// Mode returns the mode from a FileInfo structure
func (f *FileInfo) Mode() os.FileMode { return f.ModeVar }

// ModTime returns the modification time from a FileInfo structure
func (f *FileInfo) ModTime() time.Time { return time.Unix(0, f.ModTimeVar) }

// IsDir returns the is-directory indicator from a FileInfo structure
func (f *FileInfo) IsDir() bool { return f.IsDirVar }

// Sys provides an interface to a FileInfo structure
func (f *FileInfo) Sys() interface{} { return nil }

// FileHeader is a header for remote *os.File operations for remotefs.OpenFile
type FileHeader struct {
	Cmd  uint32
	Size uint64
}

const (
	Read      uint32 = iota // Read request command
	Write                   // Write request command
	Close                   // Close request command
	CmdOK                   // CmdOK is a response meaning request succeeded
	CmdFailed               // CmdFailed is a response meaning request failed.
)
