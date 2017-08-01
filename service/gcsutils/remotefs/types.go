package remotefs

import (
	"os"
	"time"
)

// ExportedError is the serialized version of the a Go error.
// It also provides a trivial implementation of the error interface.
type ExportedError struct {
	ErrString string
	ErrNum    int `json:"omitempty"`
}

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

func (f *FileInfo) Name() string       { return f.NameVar }
func (f *FileInfo) Size() int64        { return f.SizeVar }
func (f *FileInfo) Mode() os.FileMode  { return f.ModeVar }
func (f *FileInfo) ModTime() time.Time { return time.Unix(0, f.ModTimeVar) }
func (f *FileInfo) IsDir() bool        { return f.IsDirVar }
func (f *FileInfo) Sys() interface{}   { return nil }
