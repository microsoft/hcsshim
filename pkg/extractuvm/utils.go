//go:build windows
// +build windows

package extractuvm

import (
	"archive/tar"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/pkg/ociwclayer"
)

type pendingLink struct {
	name   string
	target string
}

func resolveLink(files map[string]*tar.Header, target string) (string, error) {
	var (
		resolvedFile *tar.Header
		hasFile      bool
	)
	for {
		if resolvedFile, hasFile = files[target]; !hasFile {
			return "", os.ErrNotExist
		} else if resolvedFile.Typeflag == tar.TypeReg {
			break
		} else if resolvedFile.Typeflag == tar.TypeLink {
			target = resolvedFile.Linkname
		} else {
			return "", fmt.Errorf("resolved link [%s] to invalid type: %d", target, resolvedFile.Typeflag)
		}
	}
	return resolvedFile.Name, nil
}

func hasUtilityVMFilesPrefix(path string) bool {
	// When iterating the tar, file paths show up with forward slash (`/`), however,
	// constants in wclayer use backward slash (`\`) So we have this special function to always combine with `/`
	// Also, note that the ending slash is considered to be a part of the prefix, otherwise just `UtilityVM\Files` would
	// return true, but that directory shouldn't be included
	return strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash(wclayer.UtilityVMFilesPath+`\`))
}

func hasFilesPrefix(path string) bool {
	// When iterating the tar, file paths show up with forward slash (`/`), however,
	// constants in wclayer use backward slash (`\`) So we have this special function to always combine with `/`
	// Also, note that the ending slash is considered to be a part of the prefix, otherwise just `UtilityVM\Files` would
	// return true, but that directory shouldn't be included
	return strings.HasPrefix(filepath.ToSlash(path), filepath.ToSlash("Files"+`\`))
}

func cutUtilityVMFilesPrefix(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(wclayer.UtilityVMFilesPath+`\`))
}

func validateFileType(hdr *tar.Header) error {
	if strings.HasPrefix(path.Base(hdr.Name), ociwclayer.WhiteoutPrefix) {
		return fmt.Errorf("found unexpected whiteout file [%s] in the base tar", hdr.Name)
	}
	if strings.Contains(hdr.Name, ":") {
		return fmt.Errorf("found unexpected alternate data stream file [%s] in the base tar", hdr.Name)
	}
	if hdr.Typeflag == tar.TypeSymlink {
		return fmt.Errorf("found unexpected symlink [%s] in the base tar", hdr.Name)
	}
	return nil
}
