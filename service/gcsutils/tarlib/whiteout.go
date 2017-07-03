package tarlib

import (
	"archive/tar"
	"fmt"
	"github.com/Microsoft/opengcs/service/gcsutils/fs"
	"github.com/docker/docker/pkg/archive"
	"path/filepath"
	"strings"
)

func CalcWhiteoutSize(hdr *tar.Header, f fs.Filesystem, format archive.WhiteoutFormat) (bool, error) {
	var err error

	switch format {
	case archive.AUFSWhiteoutFormat:
		return calcAUFSSize(hdr, f)
	case archive.OverlayWhiteoutFormat:
		return calcOverlaySize(hdr, f)
	default:
		return false, fmt.Errorf("unkown whiteout format: %d", err)
	}
}

func calcAUFSSize(hdr *tar.Header, f fs.Filesystem) (bool, error) {
	// Aufs uses a regular 0 size file for both.
	if strings.HasPrefix(hdr.Name, archive.WhiteoutPrefix) {
		return true, f.CalcRegFileSize(hdr.Name, 0)
	}
	return false, nil
}

func calcOverlaySize(hdr *tar.Header, f fs.Filesystem) (bool, error) {
	if archive.WhiteoutOpaqueDir == filepath.Base(hdr.Name) {
		// Aufs uses a .wh..wh..opq file inside a directory to mark it as
		// deleted. Overlayfs doesn't use this file, so we don't include it.
		// However it sets the directory's xattr, so we set that.
		dir := filepath.Dir(hdr.Name)
		return true, f.CalcAddExAttrSize(dir, "trusted.overlay.opaque", []byte{'y'}, 0)
	}

	if strings.HasPrefix(hdr.Name, archive.WhiteoutPrefix) {
		// Overlayfs uses a 0/0 character device, instead of a regular file
		return true, f.CalcCharDeviceSize(hdr.Name, 0, 0)
	}
	return false, nil
}
