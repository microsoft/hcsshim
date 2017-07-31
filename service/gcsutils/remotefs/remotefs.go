package remotefs

import (
	"errors"
	"io"
)

// ErrInvalid is returned if the parameters are invalid
var ErrInvalid = errors.New("invalid arguments")

// Func is the function definition for a generic remote fs function
// The input to the function is any serialized structs / data from in and the string slice
// from args. The output of the function will be serialized and written to out.
type Func func(stdin io.Reader, stdout io.Writer, args []string) error

// RemotefsCmd is the name of the remotefs meta command
const RemotefsCmd = "remotefs"

// Name of the commands when called from the cli context (remotefs <CMD> ...)
const (
	StatCmd           = "stat"
	LstatCmd          = "lstat"
	ReadlinkCmd       = "readlink"
	MkdirCmd          = "mkdir"
	MkdirAllCmd       = "mkdirall"
	RemoveCmd         = "remove"
	RemoveAllCmd      = "removeall"
	LinkCmd           = "link"
	SymlinkCmd        = "symlink"
	LchmodCmd         = "lchmod"
	LchownCmd         = "lchown"
	MknodCmd          = "mknod"
	MkfifoCmd         = "mkfifo"
	ReadFileCmd       = "readfile"
	WriteFileCmd      = "writefile"
	ResolvePathCmd    = "resolvepath"
	ExtractArchiveCmd = "extractarchive"
	ArchivePathCmd    = "archivepath"
)

// Commands provide a string -> remotefs function mapping.
// This is useful for commandline programs that will receive a string
// as the function to execute.
var Commands = map[string]Func{
	StatCmd:           Stat,
	LstatCmd:          Lstat,
	ReadlinkCmd:       Readlink,
	MkdirCmd:          Mkdir,
	MkdirAllCmd:       MkdirAll,
	RemoveCmd:         Remove,
	RemoveAllCmd:      RemoveAll,
	LinkCmd:           Link,
	SymlinkCmd:        Symlink,
	LchmodCmd:         Lchmod,
	LchownCmd:         Lchown,
	MknodCmd:          Mknod,
	MkfifoCmd:         Mkfifo,
	ReadFileCmd:       ReadFile,
	WriteFileCmd:      WriteFile,
	ResolvePathCmd:    ResolvePath,
	ExtractArchiveCmd: ExtractArchive,
	ArchivePathCmd:    ArchivePath,
}
