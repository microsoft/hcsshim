package commoncli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/archive"

	"github.com/Microsoft/opengcs/service/gcsutils/fs"
	"github.com/Microsoft/opengcs/service/gcsutils/libtar2vhd"
	"github.com/Microsoft/opengcs/service/gcsutils/vhd"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
)

func SetFlagsForTar2VHDLib() []*string {
	filesystem := flag.String("fs", "ext4", "Filesystem format: ext4")
	whiteout := flag.String("whiteout", "overlay", "Whiteout format: aufs, overlay")
	vhdFormat := flag.String("vhd", "fixed", "VHD format: fixed")
	tempDirectory := flag.String("tmpdir", "/mnt/gcs/LinuxServiceVM/scratch", "Temp directory for intermediate files.")
	return []*string{filesystem, whiteout, vhdFormat, tempDirectory}
}

func SetupTar2VHDLibOptions(args ...*string) (*libtar2vhd.Options, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("Mistmatched arguments for tar2vhd")
	}

	fsys := *args[0]
	wh := *args[1]
	vhdFormat := *args[2]
	tmpdir := *args[3]

	var format archive.WhiteoutFormat
	if fsys != "ext4" {
		return nil, fmt.Errorf("Unknown filesystem: %s", fsys)
	}

	if wh == "overlay" {
		format = archive.OverlayWhiteoutFormat
	} else if wh == "aufs" {
		format = archive.AUFSWhiteoutFormat
	} else {
		return nil, fmt.Errorf("Unknown whiteout format: %s", wh)
	}

	if vhdFormat != "fixed" {
		return nil, fmt.Errorf("Unknown vhd format: %s", vhdFormat)
	}

	// Note due to the semantics of MkdirAll, err == nil if the directory exists.
	if err := os.MkdirAll(tmpdir, 0755); err != nil {
		return nil, err
	}

	options := &libtar2vhd.Options{
		TarOpts:       &archive.TarOptions{WhiteoutFormat: format},
		Filesystem:    &fs.Ext4Fs{BlockSize: 4096, InodeSize: 256},
		Converter:     vhd.NewFixedVHDConverter(),
		TempDirectory: tmpdir,
	}

	return options, nil
}

func SetFlagsForLogging() []*string {
	basename := filepath.Base(os.Args[0]) + ".log"
	loggingLocation := flag.String("logfile", filepath.Join("/tmp", basename), "logging file location")
	return []*string{loggingLocation}
}

func SetupLogging(args ...*string) error {
	if len(args) < 1 {
		return fmt.Errorf("Invalid log params")
	}
	utils.SetLoggingOptions("verbose", *args[0])
	return nil
}
