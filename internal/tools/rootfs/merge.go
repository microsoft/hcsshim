// Tool to merge Windows and Linux rootfs.tar(.gz) and delta.tar (or other files) into
// a unified rootfs (gzipped) TAR.

package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// Note:
// file modification time when booting from initrd.img is the uVM boot time,
// since the kernel creates a rootfs (ramfs or tempfs) filesystem, instead of using the
// cpio archive directly.
//
// for rootfs.vhd, it is the original time as provided by the original tarball, since
// the VHD has an already-formatted (ext4) filesystem on it
//
// for the rootfs.vhd and intermediary tar and cpio archives, if they are created after
// extracting files to disk, the modification time for directories and symlinks will be
// the extraction date (and not the original date from the source).

type mergeCommand struct {
	// Append a `/` to directory names to be consistent with what GNU and BSD tar does.
	TrailingSlash bool
	// Set file and directory owner user and group ID to 0 (root) and remove user and group name.
	OverrideOwner bool
	// Prepend `./` to all paths.
	RelativePathPrefix bool
	// Override the tar header format.
	OverrideTarFormat tar.Format

	// Target OS
	OS osType

	// Paths to merge
	Paths []string

	// Compress output file
	GZIP bool
	// Destination tar file
	Dest cli.Path
}

func newMergeCommand() (*cli.Command, error) {
	c := &mergeCommand{
		OverrideTarFormat: tar.FormatPAX,
	}

	cmd := &cli.Command{
		Name:    "merge",
		Aliases: []string{"m"},
		Usage:   "merge together multiple layer tarballs",
		Description: strings.ReplaceAll(
			`Merge (tar) layers without needing to extract and combine them on the file system.
This allows preserving file properties (e.g., creation date, owner user and group) which could
be changed by extraction.`, "\n", " "),
		Args:      true,
		ArgsUsage: "layers...",
		Flags: []cli.Flag{
			&cli.PathFlag{
				Name:     "output",
				Aliases:  []string{"o"},
				Usage:    "output tarball `file`",
				Required: true,
				Action: func(_ *cli.Context, p cli.Path) (err error) {
					c.Dest, err = outputFilePath(p)
					return
				},
			},
			&cli.BoolFlag{
				Name:        "trailing-slash",
				Usage:       "append a trailing slash (/) to directories",
				Destination: &c.TrailingSlash,
			},
			&cli.BoolFlag{
				Name:        "override-owner",
				Usage:       "set file owner UID and GID to 0",
				Destination: &c.OverrideOwner,
			},
			&cli.BoolFlag{
				Name:        "relative-prefix",
				Usage:       "prefix all file paths with './'",
				Destination: &c.RelativePathPrefix,
			},
			&cli.BoolFlag{
				Name:        "gzip",
				Aliases:     []string{"z"},
				Usage:       "gzip compress output tarball",
				Destination: &c.GZIP,
			},
			&cli.StringFlag{
				Name:  "os",
				Usage: "target OS for the resulting rootfs",
				Value: string(linuxOS),
				Action: func(_ *cli.Context, s string) (err error) {
					c.OS, err = parseOSType(s)
					return
				},
			},
		},

		Before: func(cCtx *cli.Context) (err error) {
			args := cCtx.Args()
			if args.Len() < 1 {
				return fmt.Errorf("no layers specified")
			}

			c.Paths = make([]string, 0, args.Len())
			for _, s := range args.Slice() {
				p, err := filepath.Abs(s)
				if err != nil {
					return fmt.Errorf("invalid layer file path %q: %w", s, err)
				}
				c.Paths = append(c.Paths, p)
			}

			return nil
		},

		Action: c.run,
	}
	return cmd, nil
}

// basically crane (github.com/google/go-containerregistry/cmd/crane) append and export
func (c *mergeCommand) run(_ *cli.Context) error {
	logrus.WithFields(logrus.Fields{
		"trailing-slash":  c.TrailingSlash,
		"override-owner":  c.OverrideOwner,
		"relative-prefix": c.RelativePathPrefix,
		"override-format": c.OverrideTarFormat.String(),
		"os":              string(c.OS),
		"layers":          strings.Join(c.Paths, ", "),
		"gzip":            c.GZIP,
		"output":          c.Dest,
	}).Info("merging layers")

	layers := make([]v1.Layer, 0, len(c.Paths))
	for _, p := range c.Paths {
		l, err := tarball.LayerFromFile(p, tarball.WithMediaType(types.OCILayer))
		if err != nil {
			return fmt.Errorf("create layer from %q: %w", p, err)
		}
		layers = append(layers, l)
	}

	var wc io.WriteCloser
	wc, err := os.Create(c.Dest)
	if err != nil {
		return fmt.Errorf("create output file %q: %w", c.Dest, err)
	}
	defer wc.Close()

	if c.GZIP {
		wc = gzip.NewWriter(wc)
		defer wc.Close()
	}

	logrus.Trace("append layers to empty OCI image")
	if err := c.mergeLayers(wc, layers); err != nil {
		return fmt.Errorf("write merged layers to %q: %w", c.Dest, err)
	}

	logrus.WithFields(logrus.Fields{
		"output": c.Dest,
		"layers": c.Paths,
	}).Info("merged layer tarball")
	return nil
}

// merge combines multiple tar filesystems (image layers) together, and writes the result to w.
//
// Adapted from [containerregistry extract], with several key differences:
//   - operate on layers directly, without needing intermediary image
//   - use [path] for path maniputlation instead of [filepath], so files are handled consistently on Windows and Linux
//   - allow overriding non-root (non-0) file ownership, which may have been copied during tar creation
//   - allow appending a trailing slash to directories, similar to GNU and BSD tar(.exe)
//   - allow prepending a leading `./` to all files
//
// [containerregistry extract]: https://github.com/google/go-containerregistry/blob/a07d1cab8700a9875699d2e7052f47acec30399d/pkg/v1/mutate/mutate.go#L264
func (c *mergeCommand) mergeLayers(w io.Writer, layers []v1.Layer) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	// track the files we've seen before
	fileMap := make(map[string]bool)

	stats := layerWriteStats{}
	defer func() {
		logrus.WithFields(logrus.Fields{
			"total-files":            stats.totalFiles(),
			"files":                  stats.numFiles,
			"directories":            stats.numDir,
			"hard-links":             stats.numHard,
			"sym-links":              stats.numSym,
			"other-files":            stats.numOther,
			"trailing-slash-appends": stats.numTrailing,
			"relative-path-preprend": stats.numRelPrepend,
			"owner-overrides":        stats.numOwnerOverride,
			"format-overrides":       stats.numFormatOverride,
		}).Debug("finished processing image layers")
	}()

	// iterate through the layers in reverse order because it makes handling
	// whiteout layers more efficient, since we can just keep track of the removed
	// files as we see .wh. layers and ignore those in previous layers.
	for i := len(layers) - 1; i >= 0; i-- {
		layer := layers[i]
		if err := c.writeLayer(tw, &stats, fileMap, layer); err != nil {
			return err
		}
	}
	return nil

}

// append the tar layer to w.
//
// based off of:
//   - https://github.com/google/go-containerregistry/blob/a07d1cab8700a9875699d2e7052f47acec30399d/pkg/v1/mutate/mutate.go#L264
func (c *mergeCommand) writeLayer(
	tw *tar.Writer,
	stats *layerWriteStats,
	fileMap map[string]bool,
	layer v1.Layer,
) error {
	const whiteoutPrefix = ".wh."

	r, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("uncompressing layer contents: %w", err)
	}
	defer r.Close()
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return fmt.Errorf("reading layer tar: %w", err)
		}

		entry := logrus.WithFields(logrus.Fields{
			"directory": header.FileInfo().IsDir(),
			"name":      header.Name,
		})

		header.Name = c.normalize(header.Name)
		basename := path.Base(header.Name)
		dirname := path.Dir(header.Name)
		tombstone := strings.HasPrefix(basename, whiteoutPrefix)
		if tombstone {
			basename = basename[len(whiteoutPrefix):]
		}

		// check if we have seen value before
		// if we're checking a directory, don't filepath.Join names
		var name string
		if header.Typeflag == tar.TypeDir {
			name = header.Name
		} else {
			name = path.Join(dirname, basename)
		}

		if _, ok := fileMap[name]; ok {
			continue
		}

		// check for a whited out parent directory
		if inWhiteoutDir(fileMap, name) {
			continue
		}

		// update header (as needed)

		if c.RelativePathPrefix && !strings.HasPrefix(header.Name, `./`) && (c.OS != windowsOS || !strings.HasPrefix(header.Name, `.\`)) {
			stats.numRelPrepend++
			entry.Trace("prepend relative path specifier to path")

			header.Name = `./` + header.Name
		}

		switch header.Typeflag {
		case tar.TypeReg:
			stats.numFiles++
		case tar.TypeDir:
			stats.numDir++

			if c.TrailingSlash && !strings.HasSuffix(header.Name, `/`) && (c.OS != windowsOS || !strings.HasSuffix(header.Name, `\`)) {
				stats.numTrailing++
				entry.Trace("append trailing slash to directory name")

				header.Name += `/`
			}
		case tar.TypeLink:
			stats.numHard++
		case tar.TypeSymlink:
			stats.numSym++
		default:
			stats.numOther++
		}

		if c.OverrideOwner && (header.Gid != 0 || header.Gname != "" || header.Uid != 0 || header.Uname != "") {
			stats.numOwnerOverride++
			entry.WithFields(logrus.Fields{
				"group":     header.Gid,
				"groupname": header.Gname,
				"user":      header.Uid,
				"username":  header.Uname,
			}).Trace("set user and group ownership to 0 (root)")

			header.Gid = 0
			header.Uid = 0
			header.Gname = ""
			header.Uname = ""
		}

		if c.OverrideTarFormat != tar.FormatUnknown && header.Format != c.OverrideTarFormat {
			stats.numFormatOverride++
			entry.WithFields(logrus.Fields{
				"format":   header.Format.String(),
				"override": c.OverrideTarFormat.String(),
			}).Trace("override tar format")

			header.Format = c.OverrideTarFormat
		}

		// mark file as handled. non-directory implicitly tombstones
		// any entries with a matching (or child) name
		fileMap[name] = tombstone || header.Typeflag != tar.TypeDir
		if !tombstone {
			if err := tw.WriteHeader(header); err != nil {
				return fmt.Errorf("write %q header: %w", header.Name, err)
			}
			if header.Size > 0 {
				if _, err := io.CopyN(tw, tr, header.Size); err != nil {
					return fmt.Errorf("write %q contents: %w", header.Name, err)
				}
			}
		}
	}
}

// normalize names to avoid duplicates by calling [path.Clean], then removing leading and trailing slashes
func (c *mergeCommand) normalize(p string) string {
	// don't use [filepath.Clean], since that converts to "\" to "/" on Windows, which breaks Linux images
	cs := "/"
	if c.OS == windowsOS {
		cs += `\`
	}
	// triming trailing "/" should be redundant, but needed for "\" in Windows layers
	return strings.Trim(path.Clean(p), cs)
}

// based off of:
//   - https://github.com/google/go-containerregistry/blob/a07d1cab8700a9875699d2e7052f47acec30399d/pkg/v1/mutate/mutate.go#L264
func inWhiteoutDir(fileMap map[string]bool, file string) bool {
	for file != "" {
		dirname := path.Dir(file)
		if file == dirname {
			break
		}
		if val, ok := fileMap[dirname]; ok && val {
			return true
		}
		file = dirname
	}
	return false
}

func outputFilePath(s string) (string, error) {
	p, err := filepath.Abs(s)
	if err != nil {
		return "", fmt.Errorf("invalid output file path %q: %w", s, err)
	}

	entry := logrus.WithField("output", p)
	// if path is a (normal) file, it'll be silently overwritten
	if st, err := os.Stat(p); err == nil {
		if st.IsDir() {
			return "", fmt.Errorf("output file %q is a directory", p)
		} else {
			entry.Warn("existing output file will be overwritten")
		}
	} else if !os.IsNotExist(err) {
		// something weird happened, warn and hope the error goes away when we create it
		entry.WithError(err).Warn("unable to stat")
	}

	entry.Debug("using output path")
	return p, nil
}

// statistics seen when merging layers
type layerWriteStats struct {
	numTrailing       int
	numOwnerOverride  int
	numFormatOverride int
	numRelPrepend     int
	numFiles          int
	numDir            int
	numHard           int
	numSym            int
	numOther          int
}

func (x *layerWriteStats) totalFiles() int {
	return x.numFiles + x.numDir + x.numHard + x.numSym + x.numOther
}

type osType string

const (
	windowsOS = osType("windows")
	linuxOS   = osType("linux")
)

func parseOSType(s string) (t osType, _ error) {
	switch t = osType(strings.ToLower(strings.TrimSpace(s))); t {
	case windowsOS, linuxOS:
		return t, nil
	}
	return t, fmt.Errorf("unknown OS type: %s", s)
}
