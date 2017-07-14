package remotefs

import (
	"io"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/symlink"
)

// ResolvePath works like docker's symlink.FollowSymlinkInScope.
// It takens in a `path` and a `root` and evaluates symlinks in `path`
// as if they were scoped in `root`. `path` must be a child path of `root`.
// In other words, `path` must have `root` as a prefix.
// Example:
// path=/foo/bar -> /baz
// root=/foo,
// Expected result = /foo/baz
//
// Args:
// - args[0] is `path`
// - args[1] is `root`
// Out:
// - Write resolved path to stdout
func ResolvePath(in io.Reader, out io.Writer, args []string) error {
	if len(args) < 2 {
		return ErrInvalid
	}
	res, err := symlink.FollowSymlinkInScope(args[0], args[1])
	if err != nil {
		return err
	}
	if _, err = out.Write([]byte(res)); err != nil {
		return err
	}
	return nil
}

// ExtractArchive extracts the archive read from in.
// Args:
// - in = size of json | json of archive.TarOptions | input tar stream
// - args[0] = extract directory name
func ExtractArchive(in io.Reader, out io.Writer, args []string) error {
	if len(args) < 1 {
		return ErrInvalid
	}

	opts, err := ReadTarOptions(in)
	if err != nil {
		return err
	}

	if err := archive.Untar(in, args[0], opts); err != nil {
		return err
	}
	return nil
}

// ArchivePath archives the given directory and writes it to out.
// Args:
// - in = size of json | json of archive.TarOptions
// - args[0] = source directory name
// Out:
// - out = tar file of the archive
func ArchivePath(in io.Reader, out io.Writer, args []string) error {
	if len(args) < 1 {
		return ErrInvalid
	}

	opts, err := ReadTarOptions(in)
	if err != nil {
		return err
	}

	r, err := archive.TarWithOptions(args[0], opts)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, r); err != nil {
		return err
	}

	return nil
}
