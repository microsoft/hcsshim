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

const (
	StatCmd           = "stat"
	LstatCmd          = "lstat"
	MkdirAllCmd       = "mkdirall"
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
	MkdirAllCmd:       MkdirAll,
	ResolvePathCmd:    ResolvePath,
	ExtractArchiveCmd: ExtractArchive,
	ArchivePathCmd:    ArchivePath,
}
