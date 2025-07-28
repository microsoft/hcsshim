//go:build windows
// +build windows

package extractuvm

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
)

const (
	LevelTrace = slog.Level(-8)
)

func MakeUtilityVMCIMFromTar(ctx context.Context, tarPath, destPath string) (_ *cimfs.BlockCIM, err error) {
	slog.InfoContext(ctx, "Extracting UtilityVM files from tar",
		"tarPath", tarPath,
		"destPath", destPath)

	tarFile, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open layer tar: %w", err)
	}
	defer tarFile.Close()

	err = os.MkdirAll(destPath, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	uvmCIM := &cimfs.BlockCIM{
		Type:      cimfs.BlockCIMTypeSingleFile,
		BlockPath: filepath.Join(destPath, "boot.bcim"),
		CimName:   "boot.cim",
	}

	w, err := newUVMCIMWriter(ctx, uvmCIM)
	if err != nil {
		return nil, fmt.Errorf("failed to create block CIM writer: %w", err)
	}
	defer func() {
		// only attempt to close the writer in case of errors, in success case we close it anyway
		if err != nil {
			if closeErr := w.Close(ctx); closeErr != nil {
				slog.ErrorContext(ctx, "failed to close CIM writer", "error", closeErr)
			}
		}
	}()

	if err = extractUtilityVMFilesFromTar(ctx, tarFile, w); err != nil {
		return nil, fmt.Errorf("failed to extract UVM layer: %w", err)
	}

	// MUST close the writer before appending VHD footer
	if err = w.Close(ctx); err != nil {
		return nil, fmt.Errorf("failed to close CIM writer: %w", err)
	}

	// We always want to append the VHD footer to the UVM CIM
	if err := tar2ext4.ConvertFileToVhd(uvmCIM.BlockPath); err != nil {
		return nil, fmt.Errorf("failed to append VHD footer: %w", err)
	}
	return uvmCIM, nil
}

// extractUtilityVMFilesFromTar writes all the files in the tar under the
// `UitilityVM/Files` directory into the CIM.  For windows container image layer tarballs,
// there is complex web of hardlinks between the files under `Files` &
// `UtilityVM/Files`. To correctly handle this when extracting UtilityVM files this code
// makes following assumptions based on the way windows layer tarballs are currently
// structured:
// 1. If tar iteration comes across a file of type `TypeLink`, the target that this link points to MUST have already been iterated over.
// 2. When iterating over the tarball, `Files` directory tree always comes before the `UtilityVM/Files` directory.
// 3. There are hardlinks under `UtilityVM/Files` that point to files under `Files` but not vice versa.
// 4. Since this routine is supposed to be used on a base layer tarball, it doesn't expect any whiteout files in the tarball.
// 5. Files of type `TypeSymlink` are not generally used windows base layers and so the code errors out if it sees such files. Same is the case for files with alternate data streams.
// 6. There are no directories under `UtilityVM/Files` that are hardlinks to directories under `Files`. Only files can be hardlinks.
func extractUtilityVMFilesFromTar(ctx context.Context, tarFile *os.File, w *uvmCIMWriter) error {
	gr, err := gzip.NewReader(tarFile)
	if err != nil {
		return fmt.Errorf("failed to get gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// General approach:
	// Iterate over each file in the tar one by one. If we see a file that is under
	// `UtilityVM/Files`: If it is a standard file or a directory add it to the CIM
	// directly.
	// If it is a link, check the target of the link:
	// - If the target is not under `UtilityVM/Files`, save it so that we can copy the
	// target file under at the path of this link.
	// - If the target is also under UtilityVM/Files save it for later addition (we
	// can't add the link yet because the target itself could be another link, so we
	// need to wait until all link targets are resolved and added to the CIM).

	linksToAdd := []*pendingLink{}
	// a map of all the files seen in the tar - this is used to resolve nested links
	tarContents := make(map[string]*tar.Header)
	// a map of all files that we need to copy inside the UtilityVM directory from the
	// outside because there are hardlinks to it inside the UtilityVM directory.
	// There could be multiple links under UtilityVM/Files that end up directly or
	// indirectly pointing to the same target. So we may have to copy the same file at
	// multiple locations.
	// TODO (ambarve): avoid these multiple copies by only copying once and then
	// adding hardlinks.
	copies := make(map[string][]string)

	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("tar read failed: %w", err)
		}

		if err = validateFileType(hdr); err != nil {
			return err
		}

		tarContents[hdr.Name] = hdr

		if !hasUtilityVMFilesPrefix(hdr.Name) {
			continue
		}

		// At this point we either have a standard file or a link file that is
		// under the UtilityVM\Files directory.
		if hdr.Typeflag == tar.TypeLink {
			if !hasUtilityVMFilesPrefix(hdr.Linkname) {
				// link points to a file outside the UtilityVM\Files
				// directory we need to copy this file, but first resolve
				// the link
				resolvedTarget, err := resolveLink(tarContents, hdr.Linkname)
				if err != nil {
					return fmt.Errorf("failed to resolve link's [%s] target [%s]: %w", hdr.Name, hdr.Linkname, err)
				}
				copies[resolvedTarget] = append(copies[resolvedTarget], hdr.Name)
				slog.Log(ctx, LevelTrace, "adding to list of pending copies", "src", resolvedTarget, "dest", hdr.Name)
			} else {
				linksToAdd = append(linksToAdd, &pendingLink{
					name:   hdr.Name,
					target: hdr.Linkname,
				})
				slog.Log(ctx, LevelTrace, "adding to list of pending links", "link", hdr.Name, "target", hdr.Linkname)
			}
			continue
		}
		if err = w.Add(hdr, tr, false); err != nil {
			return fmt.Errorf("failed add UtilityVM standard file [%s]: %w", hdr.Name, err)
		}
		slog.Log(ctx, LevelTrace, "added standard file", "path", hdr.Name)
	}
	// close the current gzip reader before making a new one
	if err = gr.Close(); err != nil {
		return fmt.Errorf("failed to close gzip reader after first iteration: %w", err)
	}

	// reiterate tar and add copies
	if _, err = tarFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset file offset: %w", err)
	}

	gr, err = gzip.NewReader(tarFile)
	if err != nil {
		return fmt.Errorf("failed to get gzip reader: %w", err)
	}
	defer gr.Close()
	tr = tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("tar read failed: %w", err)
		}

		dsts, ok := copies[hdr.Name]
		if !ok {
			continue
		}

		if !hasFilesPrefix(hdr.Name) {
			return fmt.Errorf("copy src file doesn't have expected prefix: [%s]", hdr.Name)
		}

		for i, dst := range dsts {
			if i == 0 {
				dHdr := *hdr
				dHdr.Name = dst
				// copy the first one, next will be links to the first
				if err = w.Add(&dHdr, tr, true); err != nil {
					return fmt.Errorf("failed to copy resolved link target [%s]: %w", hdr.Name, err)
				}
			} else {
				if err = w.AddLink(dst, dsts[0], true); err != nil {
					return fmt.Errorf("failed to add links to copied file [%s]: %w", dst, err)
				}
			}
		}
	}

	for _, pl := range linksToAdd {
		if err = w.AddLink(pl.name, pl.target, false); err != nil {
			return fmt.Errorf("failed to add link from [%s] to [%s]: %w", pl.name, pl.target, err)
		}
	}
	return nil
}
