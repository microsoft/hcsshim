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
	"strings"
	"syscall"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	vhd "github.com/Microsoft/go-winio/vhd"
	"golang.org/x/sys/windows"

	"github.com/Microsoft/hcsshim/internal/winapi/cimfs"
	"github.com/Microsoft/hcsshim/internal/winapi/cimwriter"
)

func TestMain(m *testing.M) {
	fmt.Printf("cimfs.dll supported: %v\n", cimfs.Supported())
	fmt.Printf("cimwriter.dll supported: %v\n", cimwriter.Supported())
	os.Exit(m.Run())
}

// A simple tuple type used to hold information about a file/directory that is created
// during a test.
type tuple struct {
	filepath     string
	fileContents []byte
	isDir        bool
}

// a  test interface for representing both forked & block CIMs
type testCIM interface {
	// returns a full CIM path
	cimPath() string
}

type testForkedCIM struct {
	imageDir   string
	parentName string
	imageName  string
}

func (t *testForkedCIM) cimPath() string {
	return filepath.Join(t.imageDir, t.imageName)
}

type testBlockCIM struct {
	BlockCIM
}

func (t *testBlockCIM) cimPath() string {
	return filepath.Join(t.BlockPath, t.CimName)
}

type testVerifiedBlockCIM struct {
	BlockCIM
}

func (t *testVerifiedBlockCIM) cimPath() string {
	return filepath.Join(t.BlockPath, t.CimName)
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

// openNewCIM creates a new CIM and returns a writer to that CIM.  The caller MUST close
// the writer.
func openNewCIM(t *testing.T, newCIM testCIM) *CimFsWriter {
	t.Helper()

	var (
		writer *CimFsWriter
		err    error
	)

	switch val := newCIM.(type) {
	case *testForkedCIM:
		writer, err = Create(val.imageDir, val.parentName, val.imageName)
	case *testBlockCIM:
		writer, err = CreateBlockCIM(val.BlockPath, val.CimName, val.Type)
	case *testVerifiedBlockCIM:
		writer, err = CreateBlockCIMWithOptions(context.Background(), &val.BlockCIM, WithDataIntegrity())
	}
	if err != nil {
		t.Fatalf("failed while creating a cim: %s", err)
	}
	t.Cleanup(func() {
		writer.Close()
		// add 3 second sleep before test cleanup remove the cim directory
		// otherwise, that removal fails due to some handles still being open
		time.Sleep(3 * time.Second)
	})
	return writer
}

// compareContent takes in path to a directory (which is usually a volume at which a CIM is
// mounted) and ensures that every file/directory in the `testContents` shows up exactly
// as it is under that directory.
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

			// it is a file - read contents
			fileContents, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("failure while reading file %s from cim: %s", ft.filepath, err)
			} else if !bytes.Equal(fileContents, ft.fileContents) {
				t.Fatalf("contents of file %s don't match", ft.filepath)
			}
		}
	}
}

func writeCIM(t *testing.T, writer *CimFsWriter, testContents []tuple) {
	t.Helper()
	for _, ft := range testContents {
		err := createCimFileUtil(writer, ft)
		if err != nil {
			t.Fatalf("failed to create the file %s inside the cim:%s", ft.filepath, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("cim close: %s", err)
	}
}

func mountCIM(t *testing.T, testCIM testCIM, mountFlags uint32) string {
	t.Helper()
	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := Mount(testCIM.cimPath(), volumeGUID, mountFlags)
	if err != nil {
		t.Fatalf("mount cim : %s", err)
	}
	t.Cleanup(func() {
		if err := Unmount(mountvol); err != nil {
			t.Logf("CIM unmount failed: %s", err)
		}
	})
	return mountvol
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
	testCIM := &testForkedCIM{
		imageDir:   tempDir,
		parentName: "",
		imageName:  "test.cim",
	}

	writer := openNewCIM(t, testCIM)
	writeCIM(t, writer, testContents)
	mountvol := mountCIM(t, testCIM, CimMountFlagNone)
	compareContent(t, mountvol, testContents)
}

func TestBlockCIMInvalidCimName(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	blockPath := "C:\\Windows"
	cimName := ""
	_, err := CreateBlockCIM(blockPath, cimName, BlockCIMTypeSingleFile)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected error `%s`, got `%s`", err, os.ErrInvalid)
	}
}

func TestBlockCIMInvalidBlockPath(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	blockPath := ""
	cimName := "foo.bcim"
	_, err := CreateBlockCIM(blockPath, cimName, BlockCIMTypeSingleFile)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected error `%s`, got `%s", os.ErrInvalid, err)
	}
}

func TestBlockCIMInvalidType(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	blockPath := ""
	cimName := "foo.bcim"
	_, err := CreateBlockCIM(blockPath, cimName, BlockCIMTypeNone)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected error `%s`, got `%s", os.ErrInvalid, err)
	}
}

func TestCIMMergeInvalidType(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	mergedCIM := &BlockCIM{
		Type:      0,
		BlockPath: "C:\\fake\\path",
		CimName:   "fakename.cim",
	}
	// doesn't matter what we pass in the source CIM array as long as it has 2+ elements
	err := MergeBlockCIMs(mergedCIM, []*BlockCIM{mergedCIM, mergedCIM})
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected error `%s`, got `%s", os.ErrInvalid, err)
	}
}

func TestCIMMergeInvalidSourceType(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	mergedCIM := &BlockCIM{
		Type:      BlockCIMTypeDevice,
		BlockPath: "C:\\fake\\path",
		CimName:   "fakename.cim",
	}

	sCIMs := []*BlockCIM{
		{
			Type:      BlockCIMTypeDevice,
			BlockPath: "C:\\fake\\path",
			CimName:   "fakename.cim",
		},
		{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: "C:\\fake\\path",
			CimName:   "fakename.cim",
		},
	}

	// doesn't matter what we pass in the source CIM array as long as it has 2+ elements
	err := MergeBlockCIMs(mergedCIM, sCIMs)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected error `%s`, got `%s", os.ErrInvalid, err)
	}
}

func TestCIMMergeInvalidLength(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	mergedCIM := &BlockCIM{
		Type:      0,
		BlockPath: "C:\\fake\\path",
		CimName:   "fakename.cim",
	}
	err := MergeBlockCIMs(mergedCIM, []*BlockCIM{mergedCIM})
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected error `%s`, got `%s", os.ErrInvalid, err)
	}
}

func TestBlockCIMEmpty(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	root := t.TempDir()
	blockPath := filepath.Join(root, "layer.bcim")
	cimName := "layer.cim"
	w, err := CreateBlockCIM(blockPath, cimName, BlockCIMTypeSingleFile)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	err = w.Close()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestBlockCIMSingleFileReadWrite(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	root := t.TempDir()
	testCIM := &testBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: filepath.Join(root, "layer.bcim"),
			CimName:   "layer.cim",
		},
	}

	testContents := []tuple{
		{"foobar.txt", []byte("foobar test data"), false},
		{"foo", []byte(""), true},
		{"foo\\bar.txt", []byte("bar test data"), false},
	}

	writer := openNewCIM(t, testCIM)
	writeCIM(t, writer, testContents)
	mountvol := mountCIM(t, testCIM, CimMountSingleFileCim)
	compareContent(t, mountvol, testContents)
}

// creates a block device for storing a blockCIM.  returns a volume path to the block
// device that can be used for writing the CIM.
func createBlockDevice(t *testing.T, dir string) string {
	t.Helper()
	// create a VHD for storing our block CIM
	vhdPath := filepath.Join(dir, "layer.vhdx")
	if err := vhd.CreateVhdx(vhdPath, 1, 1); err != nil {
		t.Fatalf("failed to create VHD: %s", err)
	}

	diskHandle, err := vhd.OpenVirtualDisk(vhdPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		t.Fatalf("failed to open VHD: %s", err)
	}
	t.Cleanup(func() {
		closeErr := syscall.CloseHandle(diskHandle)
		if closeErr != nil {
			t.Logf("failed to close VHD handle: %s", closeErr)
		}
	})

	if err = vhd.AttachVirtualDisk(diskHandle, vhd.AttachVirtualDiskFlagNone, &vhd.AttachVirtualDiskParameters{Version: 2}); err != nil {
		t.Fatalf("failed to attach VHD: %s", err)
	}
	t.Cleanup(func() {
		detachErr := vhd.DetachVirtualDisk(diskHandle)
		if detachErr != nil {
			t.Logf("failed to detach VHD: %s", detachErr)
		}
	})

	physicalPath, err := vhd.GetVirtualDiskPhysicalPath(diskHandle)
	if err != nil {
		t.Fatalf("failed to get physical path of VHD: %s", err)
	}
	return physicalPath
}

func TestBlockCIMBlockDeviceReadWrite(t *testing.T) {
	if !IsBlockCimSupported() {
		t.Skip("blockCIM not supported on this OS version")
	}

	root := t.TempDir()

	physicalPath := createBlockDevice(t, root)

	testCIM := &testBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeDevice,
			BlockPath: physicalPath,
			CimName:   "layer.cim",
		},
	}

	testContents := []tuple{
		{"foobar.txt", []byte("foobar test data"), false},
		{"foo", []byte(""), true},
		{"foo\\bar.txt", []byte("bar test data"), false},
	}

	writer := openNewCIM(t, testCIM)
	writeCIM(t, writer, testContents)
	mountvol := mountCIM(t, testCIM, CimMountBlockDeviceCim)
	compareContent(t, mountvol, testContents)
}

func TestMergedBlockCIMs(rootT *testing.T) {
	if !IsBlockCimSupported() {
		rootT.Skipf("BlockCIM not supported")
	}

	// A slice of 3 slices, 1 slice for contents of each CIM
	testContents := [][]tuple{
		{{"foo.txt", []byte("foo1"), false}},
		{{"bar.txt", []byte("bar"), false}},
		{{"foo.txt", []byte("foo2"), false}},
	}
	// create 3 separate block CIMs
	nCIMs := len(testContents)

	// test merging for both SingleFile & BlockDevice type of block CIMs
	type testBlock struct {
		name               string
		blockType          BlockCIMType
		mountFlag          uint32
		blockPathGenerator func(t *testing.T, dir string) string
	}

	tests := []testBlock{
		{
			name:      "single file",
			blockType: BlockCIMTypeSingleFile,
			mountFlag: CimMountSingleFileCim,
			blockPathGenerator: func(t *testing.T, dir string) string {
				t.Helper()
				return filepath.Join(dir, "layer.bcim")
			},
		},
		{
			name:      "block device",
			blockType: BlockCIMTypeDevice,
			mountFlag: CimMountBlockDeviceCim,
			blockPathGenerator: func(t *testing.T, dir string) string {
				t.Helper()
				return createBlockDevice(t, dir)
			},
		},
	}

	for _, test := range tests {
		rootT.Run(test.name, func(t *testing.T) {
			sourceCIMs := make([]*BlockCIM, 0, nCIMs)
			for i := 0; i < nCIMs; i++ {
				root := t.TempDir()
				blockPath := test.blockPathGenerator(t, root)
				tc := &testBlockCIM{
					BlockCIM: BlockCIM{
						Type:      test.blockType,
						BlockPath: blockPath,
						CimName:   "layer.cim",
					}}
				writer := openNewCIM(t, tc)
				writeCIM(t, writer, testContents[i])
				sourceCIMs = append(sourceCIMs, &tc.BlockCIM)
			}

			mergedBlockPath := test.blockPathGenerator(t, t.TempDir())
			// prepare a merged CIM
			mergedCIM := &BlockCIM{
				Type:      test.blockType,
				BlockPath: mergedBlockPath,
				CimName:   "merged.cim",
			}

			if err := MergeBlockCIMs(mergedCIM, sourceCIMs); err != nil {
				t.Fatalf("failed to merge block CIMs: %s", err)
			}

			// mount and read the contents of the cim
			volumeGUID, err := guid.NewV4()
			if err != nil {
				t.Fatalf("generate cim mount GUID: %s", err)
			}

			mountvol, err := MountMergedBlockCIMs(mergedCIM, sourceCIMs, test.mountFlag, volumeGUID)
			if err != nil {
				t.Fatalf("failed to mount merged block CIMs: %s\n", err)
			}
			defer func() {
				if err := Unmount(mountvol); err != nil {
					t.Logf("CIM unmount failed: %s", err)
				}
			}()
			// since we are merging, only 1 foo.txt (from the 1st CIM) should
			// show up
			compareContent(t, mountvol, []tuple{testContents[0][0], testContents[1][0]})
		})
	}
}

func TestTombstoneInMergedBlockCIMs(rootT *testing.T) {
	if !IsBlockCimSupported() {
		rootT.Skipf("BlockCIM not supported")
	}

	root := rootT.TempDir()

	testContents := []tuple{
		{"foobar.txt", []byte("foobar test data"), false},
		{"foo", []byte(""), true},
		{"foo\\bar.txt", []byte("bar test data"), false},
	}

	cim1 := &testBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: filepath.Join(root, "1.bcim"),
			CimName:   "test.cim",
		},
	}
	writer := openNewCIM(rootT, cim1)
	writeCIM(rootT, writer, testContents)

	cim2 := &testBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: filepath.Join(root, "2.bcim"),
			CimName:   "test.cim",
		},
	}

	cim2writer := openNewCIM(rootT, cim2)

	if err := cim2writer.AddTombstone("foobar.txt"); err != nil {
		rootT.Fatalf("failed to add tombstone: %s", err)
	}
	if err := cim2writer.Close(); err != nil {
		rootT.Fatalf("failed to close the CIM: %s", err)
	}

	mergedCIM := &BlockCIM{
		Type:      BlockCIMTypeSingleFile,
		BlockPath: filepath.Join(root, "merged.cim"),
		CimName:   "merged.cim",
	}

	sourceCIMs := []*BlockCIM{&cim2.BlockCIM, &cim1.BlockCIM}
	if err := MergeBlockCIMs(mergedCIM, sourceCIMs); err != nil {
		rootT.Fatalf("failed to merge block CIMs: %s", err)
	}

	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		rootT.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := MountMergedBlockCIMs(mergedCIM, sourceCIMs, CimMountSingleFileCim, volumeGUID)
	if err != nil {
		rootT.Fatalf("failed to mount merged block CIMs: %s\n", err)
	}
	defer func() {
		if err := Unmount(mountvol); err != nil {
			rootT.Logf("CIM unmount failed: %s", err)
		}
	}()

	// verify that foobar.txt doesn't show up
	_, err = os.Stat(filepath.Join(mountvol, "foobar.txt"))
	if err == nil || !os.IsNotExist(err) {
		rootT.Fatalf("expected 'file not found' error, got: %s", err)
	}
}

func TestMergedLinksInMergedBlockCIMs(rootT *testing.T) {
	if !IsBlockCimSupported() {
		rootT.Skipf("BlockCIM not supported")
	}

	root := rootT.TempDir()

	testContents := []tuple{
		{"foobar.txt", []byte("foobar test data"), false},
		{"foo", []byte(""), true},
		{"foo\\bar.txt", []byte("bar test data"), false},
	}

	cim1 := &testBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: filepath.Join(root, "1.bcim"),
			CimName:   "test.cim",
		},
	}
	writer := openNewCIM(rootT, cim1)
	writeCIM(rootT, writer, testContents)

	cim2 := &testBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: filepath.Join(root, "2.bcim"),
			CimName:   "test.cim",
		},
	}

	cim2writer := openNewCIM(rootT, cim2)

	if err := cim2writer.AddMergedLink("foobar.txt", "b_link.txt"); err != nil {
		rootT.Fatalf("failed to add merged link: %s", err)
	}
	if err := cim2writer.AddMergedLink("b_link.txt", "a_link.txt"); err != nil {
		rootT.Fatalf("failed to add merged link: %s", err)
	}
	if err := cim2writer.Close(); err != nil {
		rootT.Fatalf("failed to close the CIM: %s", err)
	}

	mergedCIM := &BlockCIM{
		Type:      BlockCIMTypeSingleFile,
		BlockPath: filepath.Join(root, "merged.cim"),
		CimName:   "merged.cim",
	}

	sourceCIMs := []*BlockCIM{&cim2.BlockCIM, &cim1.BlockCIM}
	if err := MergeBlockCIMs(mergedCIM, sourceCIMs); err != nil {
		rootT.Fatalf("failed to merge block CIMs: %s", err)
	}

	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		rootT.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := MountMergedBlockCIMs(mergedCIM, sourceCIMs, CimMountSingleFileCim, volumeGUID)
	if err != nil {
		rootT.Fatalf("failed to mount merged block CIMs: %s\n", err)
	}
	defer func() {
		if err := Unmount(mountvol); err != nil {
			rootT.Logf("CIM unmount failed: %s", err)
		}
	}()

	// read contents of "a_link.txt", they should match that of "foobar.txt"
	data, err := os.ReadFile(filepath.Join(mountvol, "a_link.txt"))
	if err != nil {
		rootT.Logf("read file failed: %s", err)
	}
	if !bytes.Equal(data, testContents[0].fileContents) {
		rootT.Logf("file contents don't match!")
	}
}

func TestVerifiedSingleFileBlockCIMMount(t *testing.T) {
	if !IsVerifiedCimSupported() {
		t.Skipf("verified CIMs are not supported")
	}

	// contents to write to the CIM
	testContents := []tuple{
		{"foo.txt", []byte("foo1"), false},
		{"bar.txt", []byte("bar"), false},
	}

	root := t.TempDir()
	blockPath := filepath.Join(root, "layer.bcim")
	tc := &testVerifiedBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: blockPath,
			CimName:   "layer.cim",
		}}
	writer := openNewCIM(t, tc)
	writeCIM(t, writer, testContents)

	rootHash, err := GetVerificationInfo(blockPath)
	if err != nil {
		t.Fatalf("failed to get verification info: %s", err)
	}

	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := MountVerifiedBlockCIM(&tc.BlockCIM, CimMountSingleFileCim, volumeGUID, rootHash)
	if err != nil {
		t.Fatalf("mount verified cim : %s", err)
	}
	t.Cleanup(func() {
		if err := Unmount(mountvol); err != nil {
			t.Logf("CIM unmount failed: %s", err)
		}
	})
	compareContent(t, mountvol, testContents)
}

func TestVerifiedSingleFileBlockCIMMountReadFailure(t *testing.T) {
	if !IsVerifiedCimSupported() {
		t.Skipf("verified CIMs are not supported")
	}

	// contents to write to the CIM
	testContents := []tuple{
		{"foo.txt", []byte("foo1"), false},
		{"bar.txt", []byte("bar"), false},
	}

	root := t.TempDir()
	blockPath := filepath.Join(root, "layer.bcim")
	tc := &testVerifiedBlockCIM{
		BlockCIM: BlockCIM{
			Type:      BlockCIMTypeSingleFile,
			BlockPath: blockPath,
			CimName:   "layer.cim",
		}}
	writer := openNewCIM(t, tc)
	writeCIM(t, writer, testContents)

	rootHash, err := GetVerificationInfo(blockPath)
	if err != nil {
		t.Fatalf("failed to get verification info: %s", err)
	}

	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	// change the rootHash slightly, this may cause the mount to fail due to integrity check.
	rootHash[0] = rootHash[0] + 1

	_, err = MountVerifiedBlockCIM(&tc.BlockCIM, CimMountSingleFileCim, volumeGUID, rootHash)
	if err == nil {
		t.Fatalf("mount verified cim should fail with integrity error")
	} else if !strings.Contains(err.Error(), "integrity violation") {
		t.Fatalf("expected integrity violation error")
	}
}

func TestMergedVerifiedBlockCIMs(rootT *testing.T) {
	if !IsVerifiedCimSupported() {
		rootT.Skipf("verified BlockCIMs are not supported")
	}

	// A slice of 3 slices, 1 slice for contents of each CIM
	testContents := [][]tuple{
		{{"foo.txt", []byte("foo1"), false}},
		{{"bar.txt", []byte("bar"), false}},
		{{"foo.txt", []byte("foo2"), false}},
	}
	// create 3 separate block CIMs
	nCIMs := len(testContents)

	// test merging for both SingleFile & BlockDevice type of block CIMs
	type testBlock struct {
		name               string
		blockType          BlockCIMType
		mountFlag          uint32
		blockPathGenerator func(t *testing.T, dir string) string
	}

	tests := []testBlock{
		{
			name:      "single file",
			blockType: BlockCIMTypeSingleFile,
			mountFlag: CimMountSingleFileCim,
			blockPathGenerator: func(t *testing.T, dir string) string {
				t.Helper()
				return filepath.Join(dir, "layer.bcim")
			},
		},
		{
			name:      "block device",
			blockType: BlockCIMTypeDevice,
			mountFlag: CimMountBlockDeviceCim,
			blockPathGenerator: func(t *testing.T, dir string) string {
				t.Helper()
				return createBlockDevice(t, dir)
			},
		},
	}

	for _, test := range tests {
		rootT.Run(test.name, func(t *testing.T) {
			sourceCIMs := make([]*BlockCIM, 0, nCIMs)
			for i := 0; i < nCIMs; i++ {
				root := t.TempDir()
				blockPath := test.blockPathGenerator(t, root)
				tc := &testVerifiedBlockCIM{
					BlockCIM: BlockCIM{
						Type:      test.blockType,
						BlockPath: blockPath,
						CimName:   "layer.cim",
					}}
				writer := openNewCIM(t, tc)
				writeCIM(t, writer, testContents[i])
				sourceCIMs = append(sourceCIMs, &tc.BlockCIM)
			}

			mergedBlockPath := test.blockPathGenerator(t, t.TempDir())
			// prepare a merged CIM
			mergedCIM := &BlockCIM{
				Type:      test.blockType,
				BlockPath: mergedBlockPath,
				CimName:   "merged.cim",
			}

			if err := MergeBlockCIMsWithOpts(context.Background(), mergedCIM, sourceCIMs, WithDataIntegrity()); err != nil {
				t.Fatalf("failed to merge block CIMs: %s", err)
			}

			rootHash, err := GetVerificationInfo(mergedBlockPath)
			if err != nil {
				t.Fatalf("failed to get verification info: %s", err)
			}

			// mount and read the contents of the cim
			volumeGUID, err := guid.NewV4()
			if err != nil {
				t.Fatalf("generate cim mount GUID: %s", err)
			}

			mountvol, err := MountMergedVerifiedBlockCIMs(mergedCIM, sourceCIMs, test.mountFlag, volumeGUID, rootHash)
			if err != nil {
				t.Fatalf("failed to mount merged block CIMs: %s\n", err)
			}
			defer func() {
				if err := Unmount(mountvol); err != nil {
					t.Logf("CIM unmount failed: %s", err)
				}
			}()
			// since we are merging, only 1 foo.txt (from the 1st CIM) should
			// show up
			compareContent(t, mountvol, []tuple{testContents[0][0], testContents[1][0]})
		})
	}
}
