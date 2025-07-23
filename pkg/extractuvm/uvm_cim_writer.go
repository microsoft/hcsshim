//go:build windows
// +build windows

package extractuvm

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/backuptar"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"golang.org/x/sys/windows"
)

// handles the task of writing all `UtilityVM/Files/*` files into a CIM. Removes the
// `UtilityVM/Files` prefix from the file path before adding it to the CIM.
type uvmCIMWriter struct {
	cimWriter  *cimfs.CimFsWriter
	layerCIM   *cimfs.BlockCIM
	filesAdded map[string]struct{}
}

func newUVMCIMWriter(ctx context.Context, l *cimfs.BlockCIM) (*uvmCIMWriter, error) {
	var cim *cimfs.CimFsWriter
	var err error

	// We always want to create the UVM CIM with data integrity
	cim, err = cimfs.CreateBlockCIMWithOptions(ctx, l, cimfs.WithDataIntegrity())
	if err != nil {
		return nil, fmt.Errorf("error in creating a new cim: %w", err)
	}
	return &uvmCIMWriter{
		cimWriter:  cim,
		layerCIM:   l,
		filesAdded: make(map[string]struct{}),
	}, nil
}

func (u *uvmCIMWriter) ensureParents(path string) error {
	if hasUtilityVMFilesPrefix(path) {
		return fmt.Errorf("unexpected `UtilityVM/Files` prefix in the path `%s`", path)
	}

	elems := strings.Split(filepath.ToSlash(path), "/")
	currPath := ""
	for _, e := range elems[:len(elems)-1] {
		currPath = filepath.ToSlash(filepath.Join(currPath, e))
		if _, ok := u.filesAdded[currPath]; !ok {
			info := &winio.FileBasicInfo{
				FileAttributes: windows.FILE_ATTRIBUTE_DIRECTORY,
			}
			u.filesAdded[currPath] = struct{}{}
			err := u.cimWriter.AddFile(filepath.FromSlash(currPath), info, 0, nil, nil, nil)
			if err != nil {
				return fmt.Errorf("failed to ensure parent dir `%s`: %w", currPath, err)
			}
		}
	}
	return nil
}

// Add adds a file to the layer with given metadata.
func (u *uvmCIMWriter) Add(hdr *tar.Header, tr io.Reader, ensureParents bool) error {
	if !hasUtilityVMFilesPrefix(hdr.Name) {
		return fmt.Errorf("expected `UtilityVM/Files` prefix in the path `%s`", hdr.Name)
	}

	name, fileSize, fileInfo, err := backuptar.FileInfoFromHeader(hdr)
	if err != nil {
		return err
	}
	sddl, err := backuptar.SecurityDescriptorFromTarHeader(hdr)
	if err != nil {
		return err
	}
	eadata, err := backuptar.ExtendedAttributesFromTarHeader(hdr)
	if err != nil {
		return err
	}

	// we don't expect any valid reparse points here. remove the flag and pass nil byte array
	var reparse []byte
	fileInfo.FileAttributes &^= uint32(windows.FILE_ATTRIBUTE_REPARSE_POINT)

	name = cutUtilityVMFilesPrefix(name)

	if ensureParents {
		if err = u.ensureParents(name); err != nil {
			return fmt.Errorf("failed to ensure parents: %w", err)
		}
	}
	if err = u.cimWriter.AddFile(filepath.FromSlash(name), fileInfo, fileSize, sddl, eadata, reparse); err != nil {
		return err
	}
	u.filesAdded[name] = struct{}{}

	if hdr.Typeflag == tar.TypeReg {
		if _, err := io.Copy(u.cimWriter, tr); err != nil {
			return err
		}
	}
	return nil
}

// AddLink adds a hard link to the layer. The target must already have been added.
func (u *uvmCIMWriter) AddLink(name string, target string, ensureParents bool) error {
	if !hasUtilityVMFilesPrefix(name) {
		return fmt.Errorf("expected `UtilityVM/Files` prefix in the path `%s`", name)
	}
	if !hasUtilityVMFilesPrefix(target) {
		return fmt.Errorf("expected `UtilityVM/Files` prefix in the path `%s`", target)

	}
	name = cutUtilityVMFilesPrefix(name)
	target = cutUtilityVMFilesPrefix(target)
	if err := u.cimWriter.AddLink(filepath.FromSlash(target), filepath.FromSlash(name)); err != nil {
		return fmt.Errorf("failed to add link [%s] to target [%s]: %w", name, target, err)
	}
	u.filesAdded[name] = struct{}{}
	return nil
}

func (u *uvmCIMWriter) Close(ctx context.Context) error {
	return u.cimWriter.Close()
}
