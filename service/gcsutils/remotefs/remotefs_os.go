package remotefs

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
)

// TODO: @gupta-ak. Continue adding as needed to support more docker functionality.

// MkdirAll works like os.MkdirAll.
// Args:
// - args[0] is the path
// - args[1] is the permissions in octal (like 0755)
func MkdirAll(in io.Reader, out io.Writer, args []string) error {
	if len(args) < 2 {
		return ErrInvalid
	}

	perm, err := strconv.ParseUint(args[1], 8, 32)
	if err != nil {
		return err
	}
	return os.MkdirAll(args[0], os.FileMode(perm))
}

// Stat functions like os.Stat.
// Args:
// - args[0] is the path
// Out:
// - out = FileInfo object
func Stat(in io.Reader, out io.Writer, args []string) error {
	return stat(in, out, args, os.Stat)
}

// Lstat functions like os.Lstat.
// Args:
// - args[0] is the path
// Out:
// - out = FileInfo object
func Lstat(in io.Reader, out io.Writer, args []string) error {
	return stat(in, out, args, os.Lstat)
}

func stat(in io.Reader, out io.Writer, args []string, statfunc func(string) (os.FileInfo, error)) error {
	if len(args) < 1 {
		return ErrInvalid
	}

	fi, err := statfunc(args[0])
	if err != nil {
		return err
	}

	info := FileInfo{
		NameVar:    fi.Name(),
		SizeVar:    fi.Size(),
		ModeVar:    fi.Mode(),
		ModTimeVar: fi.ModTime(),
		IsDirVar:   fi.IsDir(),
	}

	buf, err := json.Marshal(info)
	if err != nil {
		return err
	}

	if _, err := out.Write(buf); err != nil {
		return err
	}

	return nil
}
