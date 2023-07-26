//go:build windows
// +build windows

package cimfs

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// A simple tuple type used to hold information about a file/directory that is created
// during a test.
type tuple struct {
	filepath     string
	fileContents []byte
	isDir        bool
}

// A utility function to create a file/directory and write data to it in the given cim
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

// This test creates a cim, writes some files to it and then reads those files back.
// The cim created by this test has only 3 files in the following tree
// /
// |- foobar.txt
// |- foo
// |--- bar.txt
func TestCimReadWrite(t *testing.T) {
	if !IsCimFsSupported() {
		t.Skipf("CimFs not supported")
	}

	testContents := []tuple{
		{"foobar.txt", []byte("foobar test data"), false},
		{"foo", []byte(""), true},
		{"foo\\bar.txt", []byte("bar test data"), false},
	}

	tempDir := t.TempDir()

	cimName := "test.cim"
	cimPath := filepath.Join(tempDir, cimName)
	c, err := Create(tempDir, "", cimName)
	if err != nil {
		t.Fatalf("failed while creating a cim: %s", err)
	}
	defer func() {
		// destroy cim sometimes fails if tried immediately after accessing & unmounting the cim so
		// give some time and then remove.
		time.Sleep(3 * time.Second)
		if err := DestroyCim(context.Background(), cimPath); err != nil {
			t.Fatalf("destroy cim failed: %s", err)
		}
	}()

	for _, ft := range testContents {
		err := createCimFileUtil(c, ft)
		if err != nil {
			t.Fatalf("failed to create the file %s inside the cim:%s", ft.filepath, err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatalf("cim close: %s", err)
	}

	// mount and read the contents of the cim
	mountvol, err := Mount(cimPath)
	if err != nil {
		t.Fatalf("mount cim : %s", err)
	}
	defer func() {
		if err := Unmount(mountvol); err != nil {
			t.Fatalf("unmount failed: %s", err)
		}
	}()

	for _, ft := range testContents {
		if ft.isDir {
			_, err := os.Stat(filepath.Join(mountvol, ft.filepath))
			if err != nil {
				t.Fatalf("stat directory %s from cim: %s", ft.filepath, err)
			}
		} else {
			f, err := os.Open(filepath.Join(mountvol, ft.filepath))
			if err != nil {
				t.Fatalf("open file %s: %s", filepath.Join(mountvol, ft.filepath), err)
			}
			defer f.Close()

			fileContents := make([]byte, len(ft.fileContents))

			// it is a file - read contents
			rc, err := f.Read(fileContents)
			if err != nil && err != io.EOF {
				t.Fatalf("failure while reading file %s from cim: %s", ft.filepath, err)
			} else if rc != len(ft.fileContents) {
				t.Fatalf("couldn't read complete file contents for file: %s, read %d bytes, expected: %d", ft.filepath, rc, len(ft.fileContents))
			} else if !bytes.Equal(fileContents[:rc], ft.fileContents) {
				t.Fatalf("contents of file %s don't match", ft.filepath)
			}
		}
	}

}
