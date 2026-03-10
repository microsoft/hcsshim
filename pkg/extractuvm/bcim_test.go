//go:build windows
// +build windows

package extractuvm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/pkg/cimfs"
)

// A simple tuple type used to hold information about a file/directory that is created
// during a test.
type testFile struct {
	hdr          *tar.Header
	fileContents []byte
}

// compareContent takes in path to a directory (which is usually a volume at which a CIM is
// mounted) and ensures that every file/directory in the `testContents` shows up exactly
// as it is under that directory.
func compareContent(t *testing.T, root string, testContents []testFile) {
	t.Helper()

	for _, ft := range testContents {
		ftPath := filepath.Join(root, ft.hdr.Name)
		if ft.hdr.Typeflag == tar.TypeDir {
			_, err := os.Stat(ftPath)
			if err != nil {
				t.Fatalf("stat directory %s from cim: %s", ftPath, err)
			}
		} else {
			f, err := os.Open(filepath.Join(root, ft.hdr.Name))
			if err != nil {
				t.Fatalf("open file %s: %s", ftPath, err)
			}
			defer f.Close()

			// it is a file - read contents
			fileContents, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("failure while reading file %s from cim: %s", ftPath, err)
			} else if !bytes.Equal(fileContents, ft.fileContents) {
				t.Fatalf("contents of file %s don't match", ftPath)
			}
		}
	}
}

func mountBlockCIM(t *testing.T, bCIM *cimfs.BlockCIM, mountFlags uint32) string {
	t.Helper()
	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := cimfs.Mount(filepath.Join(bCIM.BlockPath, bCIM.CimName), volumeGUID, mountFlags)
	if err != nil {
		t.Fatalf("mount cim : %s", err)
	}
	t.Cleanup(func() {
		if err := cimfs.Unmount(mountvol); err != nil {
			t.Logf("CIM unmount failed: %s", err)
		}
	})
	return mountvol
}

func makeGzipTar(t *testing.T, contents []testFile) string {
	t.Helper()

	// Create a temporary file to hold the tar
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "test.tar.gz")

	// open the tar file for writing
	tarFile, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("failed to create tar file: %v", err)
	}
	defer tarFile.Close()

	// Create a gzip writer
	gzipWriter := gzip.NewWriter(tarFile)
	defer gzipWriter.Close()

	// Create a tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for _, item := range contents {
		err = tarWriter.WriteHeader(item.hdr)
		if err != nil {
			t.Fatalf("failed to write dir header: %v", err)
		}
		if item.hdr.Typeflag == tar.TypeReg {
			_, err = tarWriter.Write(item.fileContents)
			if err != nil {
				t.Fatalf("failed to write file contents: %v", err)
			}
		}
	}

	err = tarWriter.Close()
	if err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}

	err = gzipWriter.Close()
	if err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	return tarPath
}

func extractAndVerifyTarToCIM(t *testing.T, tarContents []testFile, contentsToVerify []testFile) {
	t.Helper()

	if testing.Verbose() {
		originalLogger := slog.Default()
		defer func() {
			// Reset log level after test
			slog.SetDefault(originalLogger)
		}()

		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: LevelTrace,
		})))
	}

	tarPath := makeGzipTar(t, tarContents)

	// Create a temporary directory to extract the tar contents
	destDir := t.TempDir()

	// Call the function to extract the tar contents
	uvmCIM, err := MakeUtilityVMCIMFromTar(context.Background(), tarPath, destDir)
	if err != nil {
		t.Fatalf("failed to extract tar contents: %v", err)
	}
	mountedCIMVolume := mountBlockCIM(t, uvmCIM, cimfs.CimMountSingleFileCim)
	compareContent(t, mountedCIMVolume, contentsToVerify)
}

func TestTarUtilityVMExtract(t *testing.T) {
	if !cimfs.IsVerifiedCimMountSupported() {
		t.Skip("block CIMs are not supported on this build")
	}

	tarContents := []testFile{
		{
			fileContents: []byte("F1"),
			hdr: &tar.Header{
				Name:     "Files\\f1.txt",
				Size:     2,
				Typeflag: tar.TypeReg,
			},
		},
		{
			fileContents: []byte("F2"),
			hdr: &tar.Header{
				Name:     "Files\\f2.txt",
				Size:     2,
				Typeflag: tar.TypeReg,
			},
		},
		{
			// Standard file under UtilityVM\Files directory
			hdr: &tar.Header{
				Name:     "UtilityVM\\Files",
				Typeflag: tar.TypeDir,
			},
		},
		{
			fileContents: []byte("U"),
			hdr: &tar.Header{
				Name:     "UtilityVM\\Files\\u.txt",
				Size:     1,
				Typeflag: tar.TypeReg,
			},
		},
		{
			// Link under UtilityVM\Files pointing to a file under Files
			hdr: &tar.Header{
				Name:     "UtilityVM\\Files\\ul1.txt",
				Linkname: "Files\\f1.txt",
				Typeflag: tar.TypeLink,
			},
		},
		{
			// Link under UtilityVM\Files pointing to a file also under UtilityVM\Files
			hdr: &tar.Header{
				Name:     "UtilityVM\\Files\\ul2.txt",
				Linkname: "UtilityVM\\Files\\u.txt",
				Typeflag: tar.TypeLink,
			},
			fileContents: nil, // No content for links
		},
	}

	extractedContents := []testFile{
		{
			fileContents: []byte("U"),
			hdr: &tar.Header{
				Name:     "u.txt",
				Size:     1,
				Typeflag: tar.TypeReg,
			},
		},
		{
			// Link under UtilityVM\Files pointing to a file under Files
			fileContents: []byte("F1"),
			hdr: &tar.Header{
				Name: "ul1.txt",
				Size: 2,
			},
		},
		{
			fileContents: []byte("U"),
			hdr: &tar.Header{
				Name: "ul2.txt",
				Size: 1,
			},
		},
	}
	extractAndVerifyTarToCIM(t, tarContents, extractedContents)
}
