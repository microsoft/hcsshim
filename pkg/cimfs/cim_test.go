//go:build windows
// +build windows

package cimfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"golang.org/x/sys/windows"
)

// A simple tuple type used to hold information about a file/directory that is created
// during a test.
type tuple struct {
	filepath     string
	fileContents []byte
	isDir        bool
}

// A utility function to create a file/directory and write data to it in the given cim.
func createCimFileUtil(c *CimFsWriter, fileTuple tuple) error {
	// create files inside the cim
	fileInfo := &winio.FileBasicInfo{
		CreationTime:   windows.NsecToFiletime(time.Now().UnixNano()),
		LastAccessTime: windows.NsecToFiletime(time.Now().UnixNano()),
		LastWriteTime:  windows.NsecToFiletime(time.Now().UnixNano()),
		ChangeTime:     windows.NsecToFiletime(time.Now().UnixNano()),
		FileAttributes: 0,
	}
	if fileTuple.isDir {
		fileInfo.FileAttributes = windows.FILE_ATTRIBUTE_DIRECTORY
	}

	if err := c.AddFile(filepath.FromSlash(fileTuple.filepath), fileInfo, int64(len(fileTuple.fileContents)), []byte{}, []byte{}, []byte{}); err != nil {
		return err
	}

	if !fileTuple.isDir {
		wc, err := c.Write(fileTuple.fileContents)
		if err != nil || wc != len(fileTuple.fileContents) {
			if err == nil {
				return fmt.Errorf("unable to finish writing to file %s", fileTuple.filepath)
			} else {
				return err
			}
		}
	}
	return nil
}

// openNewCIM creates a new CIM inside `dirPath` with name `name` and returns a writer to that CIM.
// The caller MUST commit the CIM & close the writer.
func openNewCIM(t *testing.T, name, dirPath string) *CimFsWriter {
	t.Helper()

	cimPath := filepath.Join(dirPath, name)
	c, err := Create(dirPath, "", name)
	if err != nil {
		t.Fatalf("failed while creating a cim: %s", err)
	}
	t.Cleanup(func() {
		// destroy cim sometimes fails if tried immediately after accessing & unmounting the cim so
		// give some time and then remove.
		time.Sleep(3 * time.Second)
		if err := DestroyCim(context.Background(), cimPath); err != nil {
			t.Fatalf("destroy cim failed: %s", err)
		}
	})
	return c
}

// writeNewCIM creates a new CIM with `name` inside directory `dirPath` and writes the
// given data inside it. The CIM is then committed and closed.
func writeNewCIM(t *testing.T, name, dirPath string, contents []tuple) {
	t.Helper()

	c := openNewCIM(t, name, dirPath)
	for _, ft := range contents {
		err := createCimFileUtil(c, ft)
		if err != nil {
			t.Fatalf("failed to create the file %s inside the cim:%s", ft.filepath, err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatalf("cim close: %s", err)
	}
}

// compareContent takes in path to a directory (which is usually a volume at which a CIM is mounted) and
// ensures that every file/directory in the `testContents` shows up exactly as it is under that directory.
func compareContent(t *testing.T, root string, testContents []tuple) {
	t.Helper()

	for _, ft := range testContents {
		if ft.isDir {
			_, err := os.Stat(filepath.Join(root, ft.filepath))
			if err != nil {
				t.Fatalf("stat directory %s from cim: %s", ft.filepath, err)
			}
		} else {
			f, err := os.Open(filepath.Join(root, ft.filepath))
			if err != nil {
				t.Fatalf("open file %s: %s", filepath.Join(root, ft.filepath), err)
			}
			defer f.Close()

			fileContents := make([]byte, len(ft.fileContents))

			// it is a file - read contents
			rc, err := f.Read(fileContents)
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("failure while reading file %s from cim: %s", ft.filepath, err)
			} else if !bytes.Equal(fileContents[:rc], ft.fileContents) {
				t.Fatalf("contents of file %s don't match", ft.filepath)
			}
		}
	}
}

// This test creates a cim, writes some files to it and then reads those files back.
// The cim created by this test has only 3 files in the following tree
// /
// |- foobar.txt
// |- foo
// |--- bar.txt
func TestCimReadWrite(t *testing.T) {
	if !IsCimFSSupported() {
		t.Skipf("CimFs not supported")
	}

	testContents := []tuple{
		{"foobar.txt", []byte("foobar test data"), false},
		{"foo", []byte(""), true},
		{"foo\\bar.txt", []byte("bar test data"), false},
	}

	tempDir := t.TempDir()

	writeNewCIM(t, "test.cim", tempDir, testContents)

	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := Mount(filepath.Join(tempDir, "test.cim"), volumeGUID, hcsschema.CimMountFlagCacheFiles)
	if err != nil {
		t.Fatalf("mount cim : %s", err)
	}
	defer func() {
		if err := Unmount(mountvol); err != nil {
			t.Fatalf("unmount failed: %s", err)
		}
	}()

	compareContent(t, mountvol, testContents)
}

// This test creates two CIMs, writes some files to them, then merges those CIMs, mounts the merged CIM and reads the files back.
func TestMergedCims(t *testing.T) {
	if !IsMergedCimSupported() {
		t.Skipf("merged CIMs are not supported")
	}

	cim1Dir := t.TempDir()
	cim1Contents := []tuple{
		{"f1.txt", []byte("f1"), false},
		{"f2.txt", []byte("f2"), false},
	}
	cim1Name := "test1.cim"
	cim1Path := filepath.Join(cim1Dir, cim1Name)
	writeNewCIM(t, cim1Name, cim1Dir, cim1Contents)

	cim2Dir := t.TempDir()
	cim2Contents := []tuple{
		{"f1.txt", []byte("f1overwrite"), false}, // overwrite file from lower layer
		{"f3.txt", []byte("f3"), false},
	}
	cim2Name := "test2.cim"
	cim2Path := filepath.Join(cim2Dir, cim2Name)
	writeNewCIM(t, cim2Name, cim2Dir, cim2Contents)

	// create a merged CIM in 2nd CIMs directory
	mergedName := "testmerged.cim"
	mergedPath := filepath.Join(cim2Dir, mergedName)
	// order of CIMs in topmost first and bottom most last
	err := CreateMergedCim(cim2Dir, mergedName, []string{cim2Path, cim1Path})
	if err != nil {
		t.Fatalf("failed to merge CIMs: %s", err)
	}

	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := MountMergedCims([]string{cim1Path, cim2Path}, mergedPath, hcsschema.CimMountFlagCacheFiles, volumeGUID)
	if err != nil {
		t.Fatalf("mount cim failed: %s", err)
	}
	defer func() {
		if err := Unmount(mountvol); err != nil {
			t.Fatalf("unmount failed: %s", err)
		}
	}()

	// we expect to find f1 (overwritten), f2 & f3
	allContent := []tuple{cim1Contents[1], cim2Contents[0], cim2Contents[1]}
	compareContent(t, mountvol, allContent)
}
