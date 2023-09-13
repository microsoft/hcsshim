//go:build linux
// +build linux

package scsi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"golang.org/x/sys/unix"
)

func clearTestDependencies() {
	osReadDir = nil
	osStat = nil
	osMkdirAll = nil
	osRemoveAll = nil
	unixMount = nil
	getDevicePath = nil
	createVerityTarget = nil
	encryptDevice = nil
	cleanupCryptDevice = nil
	storageUnmountPath = nil
	_getDeviceFsType = nil
	_tar2ext4IsDeviceExt4 = nil
	ext4Format = nil
	xfsFormat = nil
}

// fakeFileInfo is a mock os.FileInfo that can be used to return
// in mock os calls
var _ fs.FileInfo = &fakeFileInfo{}

type fakeFileInfo struct {
	name string
}

func (f *fakeFileInfo) Name() string {
	return f.name
}

func (f *fakeFileInfo) Size() int64 {
	// fake size
	return 100
}

func (f *fakeFileInfo) Mode() os.FileMode {
	// fake mode
	return os.ModeDir
}

func (f *fakeFileInfo) ModTime() time.Time {
	// fake time
	return time.Now()
}

func (f *fakeFileInfo) IsDir() bool {
	// fake isDir
	return false
}

func (f *fakeFileInfo) Sys() interface{} {
	return nil
}

// fakeDirEntry is a mock os.DirEntry that can be used to return in
// the response from the mocked os.ReadDir call.
var _ os.DirEntry = &fakeDirEntry{}

type fakeDirEntry struct {
	name string
}

func (d *fakeDirEntry) Name() string {
	return d.name
}

func (d *fakeDirEntry) IsDir() bool {
	return true
}

func (d *fakeDirEntry) Type() os.FileMode {
	return os.ModeDir
}

func (d *fakeDirEntry) Info() (os.FileInfo, error) {
	return &fakeFileInfo{name: d.name}, nil
}

func osStatNoop(source string) (os.FileInfo, error) {
	return &fakeFileInfo{
		name: source,
	}, nil
}

func getDeviceFsTypeExt4(source string) (string, error) {
	return "ext4", nil
}

func getDeviceFsTypeUnknown(source string) (string, error) {
	return "", ErrUnknownFilesystem
}

func Test_Mount_Mkdir_Fails_Error(t *testing.T) {
	clearTestDependencies()

	expectedErr := errors.New("mkdir : no such file or directory")
	osMkdirAll = func(path string, perm os.FileMode) error {
		return expectedErr
	}
	_getDeviceFsType = getDeviceFsTypeExt4
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	osStat = osStatNoop

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"",
		false,
		nil,
		config,
	); !errors.Is(err, expectedErr) {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
}

func Test_Mount_Mkdir_ExpectedPath(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	target := "/fake/path"
	osMkdirAll = func(path string, perm os.FileMode) error {
		if path != target {
			t.Errorf("expected path: %v, got: %v", target, path)
			return errors.New("unexpected path")
		}
		return nil
	}
	_getDeviceFsType = getDeviceFsTypeExt4
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	osStat = osStatNoop

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		target,
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_Mkdir_ExpectedPerm(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	target := "/fake/path"
	osMkdirAll = func(path string, perm os.FileMode) error {
		if perm != os.FileMode(0700) {
			t.Errorf("expected perm: %v, got: %v", os.FileMode(0700), perm)
			return errors.New("unexpected perm")
		}
		return nil
	}
	_getDeviceFsType = getDeviceFsTypeExt4
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	osStat = osStatNoop
	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}

	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		target,
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_GetDevicePath_Valid_Controller(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedController := uint8(2)
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		if expectedController != controller {
			t.Errorf("expected controller: %v, got: %v", expectedController, controller)
			return "", errors.New("unexpected controller")
		}
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		expectedController,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_GetDevicePath_Valid_Lun(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedLun := uint8(2)
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		if expectedLun != lun {
			t.Errorf("expected lun: %v, got: %v", expectedLun, lun)
			return "", errors.New("unexpected lun")
		}
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}

	if err := Mount(
		context.Background(),
		0,
		expectedLun,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_GetDevicePath_Valid_Partition(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedPartition := uint64(3)
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		if expectedPartition != partition {
			t.Errorf("expected partition: %v, got: %v", expectedPartition, partition)
			return "", errors.New("unexpected lun")
		}
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}

	if err := Mount(
		context.Background(),
		0,
		0,
		expectedPartition,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_Calls_RemoveAll_OnMountFailure(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	target := "/fake/path"
	removeAllCalled := false
	osRemoveAll = func(path string) error {
		removeAllCalled = true
		if path != target {
			t.Errorf("expected path: %v, got: %v", target, path)
			return errors.New("unexpected path")
		}
		return nil
	}
	expectedErr := errors.New("unexpected mount failure")
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount failure to test remove is called
		return expectedErr
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		target,
		false,
		nil,
		config,
	); !errors.Is(err, expectedErr) {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
	if !removeAllCalled {
		t.Fatal("expected os.RemoveAll to be called on mount failure")
	}
}

func Test_Mount_Valid_Source(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedSource := "/dev/sdz"
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return expectedSource, nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if expectedSource != source {
			t.Errorf("expected source: %s, got: %s", expectedSource, source)
			return errors.New("unexpected source")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_Target(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	expectedTarget := "/fake/path"
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if expectedTarget != target {
			t.Errorf("expected target: %s, got: %s", expectedTarget, target)
			return errors.New("unexpected target")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		expectedTarget,
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_FSType(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFSType := "ext4"
		if expectedFSType != fstype {
			t.Errorf("expected fstype: %s, got: %s", expectedFSType, fstype)
			return errors.New("unexpected fstype")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_Flags(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFlags := uintptr(0)
		if expectedFlags != flags {
			t.Errorf("expected flags: %v, got: %v", expectedFlags, flags)
			return errors.New("unexpected flags")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Readonly_Valid_Flags(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFlags := uintptr(unix.MS_RDONLY)
		if expectedFlags != flags {
			t.Errorf("expected flags: %v, got: %v", expectedFlags, flags)
			return errors.New("unexpected flags")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		true,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_Data(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if data != "" {
			t.Errorf("expected empty data, got: %s", data)
			return errors.New("unexpected data")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Readonly_Valid_Data(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedData := "noload"
		if expectedData != data {
			t.Errorf("expected data: %s, got: %s", expectedData, data)
			return errors.New("unexpected data")
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		true,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_EnsureFilesystem_Success(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		return nil
	}
	osStat = osStatNoop
	ext4Format = func(ctx context.Context, source string) error {
		return nil
	}

	_getDeviceFsType = getDeviceFsTypeExt4
	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: true,
		Filesystem:       "ext4",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		true,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_EnsureFilesystem_Unsupported(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	getDevicePath = func(ctx context.Context, controller, lun uint8, partition uint64) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		return nil
	}
	osStat = osStatNoop
	osRemoveAll = func(string) error {
		return nil
	}
	_getDeviceFsType = getDeviceFsTypeUnknown
	config := &Config{
		Encrypted:        false,
		VerityInfo:       nil,
		EnsureFilesystem: true,
		Filesystem:       "fake",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		true,
		nil,
		config,
	); err == nil {
		t.Fatal("expected to get an error from unsupported fs type")
	}
}

// dm-verity tests

func Test_CreateVerityTarget_And_Mount_Called_With_Correct_Parameters(t *testing.T) {
	clearTestDependencies()

	expectedVerityName := fmt.Sprintf(verityDeviceFmt, 0, 0, 0, "hash")
	expectedSource := "/dev/sdb"
	expectedMapperPath := fmt.Sprintf("/dev/mapper/%s", expectedVerityName)
	expectedTarget := "/foo"
	createVerityTargetCalled := false

	getDevicePath = func(_ context.Context, _, _ uint8, _ uint64) (string, error) {
		return expectedSource, nil
	}

	osMkdirAll = func(_ string, _ os.FileMode) error {
		return nil
	}

	vInfo := &guestresource.DeviceVerityInfo{
		RootDigest: "hash",
	}
	createVerityTarget = func(_ context.Context, source, name string, verityInfo *guestresource.DeviceVerityInfo) (string, error) {
		createVerityTargetCalled = true
		if source != expectedSource {
			t.Errorf("expected source %s, got %s", expectedSource, source)
		}
		if name != expectedVerityName {
			t.Errorf("expected verity target name %s, got %s", expectedVerityName, name)
		}
		return expectedMapperPath, nil
	}

	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if source != expectedMapperPath {
			t.Errorf("expected unixMount source %s, got %s", expectedMapperPath, source)
		}
		if target != expectedTarget {
			t.Errorf("expected unixMount target %s, got %s", expectedTarget, target)
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       vInfo,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		expectedTarget,
		true,
		nil,
		config,
	); err != nil {
		t.Fatalf("unexpected error during Mount: %s", err)
	}
	if !createVerityTargetCalled {
		t.Fatalf("expected createVerityTargetCalled to be called")
	}
}

func Test_osMkdirAllFails_And_RemoveDevice_Called(t *testing.T) {
	clearTestDependencies()

	expectedError := errors.New("osMkdirAll error")
	expectedVerityName := fmt.Sprintf(verityDeviceFmt, 0, 0, 0, "hash")
	removeDeviceCalled := false

	getDevicePath = func(_ context.Context, _, _ uint8, _ uint64) (string, error) {
		return "/dev/sdb", nil
	}

	osMkdirAll = func(_ string, _ os.FileMode) error {
		return expectedError
	}

	verityInfo := &guestresource.DeviceVerityInfo{
		RootDigest: "hash",
	}

	createVerityTarget = func(_ context.Context, _, _ string, _ *guestresource.DeviceVerityInfo) (string, error) {
		return fmt.Sprintf("/dev/mapper/%s", expectedVerityName), nil
	}

	removeDevice = func(name string) error {
		removeDeviceCalled = true
		if name != expectedVerityName {
			t.Errorf("expected RemoveDevice name %s, got %s", expectedVerityName, name)
		}
		return nil
	}
	osStat = osStatNoop
	_getDeviceFsType = getDeviceFsTypeExt4

	config := &Config{
		Encrypted:        false,
		VerityInfo:       verityInfo,
		EnsureFilesystem: false,
		Filesystem:       "",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/foo",
		true,
		nil,
		config,
	); err != expectedError {
		t.Fatalf("expected Mount error %s, got %s", expectedError, err)
	}
	if !removeDeviceCalled {
		t.Fatal("expected removeDevice to be called")
	}
}

func Test_Mount_EncryptDevice_Called(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(string, os.FileMode) error {
		return nil
	}
	getDevicePath = func(context.Context, uint8, uint8, uint64) (string, error) {
		return "", nil
	}
	unixMount = func(string, string, string, uintptr, string) error {
		return nil
	}
	encryptDeviceCalled := false
	expectedCryptTarget := fmt.Sprintf(cryptDeviceFmt, 0, 0, 0)
	expectedDevicePath := "/dev/mapper/" + expectedCryptTarget

	encryptDevice = func(_ context.Context, source string, devName string) (string, error) {
		if devName != expectedCryptTarget {
			t.Fatalf("expected crypt device %q got %q", expectedCryptTarget, devName)
		}
		encryptDeviceCalled = true
		return expectedDevicePath, nil
	}
	osStat = osStatNoop

	xfsFormat = func(arg string) error {
		if arg != expectedDevicePath {
			t.Fatalf("expected args: '%v' got: '%v'", expectedDevicePath, arg)
		}
		return nil
	}

	config := &Config{
		Encrypted:        true,
		VerityInfo:       nil,
		EnsureFilesystem: true,
		Filesystem:       "xfs",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err != nil {
		t.Fatalf("expected nil error, got: %s", err)
	}
	if !encryptDeviceCalled {
		t.Fatal("expected encryptDevice to be called")
	}
}

func Test_Mount_EncryptDevice_Mkfs_Error(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(string, os.FileMode) error {
		return nil
	}
	getDevicePath = func(context.Context, uint8, uint8, uint64) (string, error) {
		return "", nil
	}
	unixMount = func(string, string, string, uintptr, string) error {
		return nil
	}
	expectedCryptTarget := fmt.Sprintf(cryptDeviceFmt, 0, 0, 0)
	expectedDevicePath := "/dev/mapper/" + expectedCryptTarget

	encryptDevice = func(_ context.Context, source string, devName string) (string, error) {
		if devName != expectedCryptTarget {
			t.Fatalf("expected crypt device %q got %q", expectedCryptTarget, devName)
		}
		return expectedDevicePath, nil
	}
	osStat = osStatNoop

	xfsFormat = func(arg string) error {
		if arg != expectedDevicePath {
			t.Fatalf("expected args: '%v' got: '%v'", expectedDevicePath, arg)
		}
		return fmt.Errorf("fake error")
	}
	osRemoveAll = func(string) error {
		return nil
	}

	config := &Config{
		Encrypted:        true,
		VerityInfo:       nil,
		EnsureFilesystem: true,
		Filesystem:       "xfs",
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	); err == nil {
		t.Fatalf("expected to fail")
	}
}

func Test_Mount_RemoveAllCalled_When_EncryptDevice_Fails(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(string, os.FileMode) error {
		return nil
	}
	getDevicePath = func(context.Context, uint8, uint8, uint64) (string, error) {
		return "", nil
	}
	unixMount = func(string, string, string, uintptr, string) error {
		return nil
	}
	encryptDeviceError := errors.New("encrypt device error")
	encryptDevice = func(context.Context, string, string) (string, error) {
		return "", encryptDeviceError
	}
	removeAllCalled := false
	osRemoveAll = func(string) error {
		removeAllCalled = true
		return nil
	}
	osStat = osStatNoop

	xfsFormat = func(arg string) error {
		return nil
	}

	config := &Config{
		Encrypted:        true,
		VerityInfo:       nil,
		EnsureFilesystem: true,
		Filesystem:       "xfs",
	}
	err := Mount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		false,
		nil,
		config,
	)
	if err == nil {
		t.Fatalf("expected to fail")
	}
	if errors.Unwrap(err) != encryptDeviceError {
		t.Fatalf("expected error %q, got %q", encryptDeviceError, err)
	}
	if !removeAllCalled {
		t.Fatal("osRemoveAll was not called")
	}
}

func Test_Unmount_CleanupCryptDevice_Called(t *testing.T) {
	clearTestDependencies()

	storageUnmountPath = func(context.Context, string, bool) error {
		return nil
	}
	cleanupCryptDeviceCalled := false
	cleanupCryptDevice = func(_ context.Context, devName string) error {
		expectedDevName := fmt.Sprintf(cryptDeviceFmt, 0, 0, 0)
		if devName != expectedDevName {
			t.Fatalf("expected crypt target %q, got %q", expectedDevName, devName)
		}
		cleanupCryptDeviceCalled = true
		return nil
	}
	osStat = osStatNoop

	config := &Config{
		Encrypted: true,
	}
	if err := Unmount(
		context.Background(),
		0,
		0,
		0,
		"/fake/path",
		config,
	); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !cleanupCryptDeviceCalled {
		t.Fatal("cleanupCryptDevice not called")
	}
}

func Test_GetDevicePath_Device_With_Partition(t *testing.T) {
	clearTestDependencies()

	deviceName := "sdd"
	partition := uint64(1)
	deviceWithPartitionName := deviceName + fmt.Sprintf("%d", partition)
	expectedDevicePath := filepath.Join("/dev", deviceWithPartitionName)

	osReadDir = func(_ string) ([]os.DirEntry, error) {
		entry := &fakeDirEntry{name: deviceName}
		return []os.DirEntry{entry}, nil
	}

	osStat = func(_ string) (os.FileInfo, error) {
		return &fakeFileInfo{
			name: deviceWithPartitionName,
		}, nil
	}

	getDevicePath = GetDevicePath

	actualPath, err := getDevicePath(context.Background(), 0, 0, partition)
	if err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}
	if actualPath != expectedDevicePath {
		t.Fatalf("expected to get %v, instead got %v", expectedDevicePath, actualPath)
	}
}

func Test_GetDevicePath_Device_With_Partition_Error(t *testing.T) {
	clearTestDependencies()

	deviceName := "sdd"
	partition := uint64(1)

	osReadDir = func(_ string) ([]os.DirEntry, error) {
		entry := &fakeDirEntry{name: deviceName}
		return []os.DirEntry{entry}, nil
	}

	osStat = func(_ string) (os.FileInfo, error) {
		return nil, nil
	}

	getDevicePath = GetDevicePath

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	actualPath, err := getDevicePath(ctx, 0, 0, partition)
	if err == nil {
		t.Fatalf("expected to get an error, instead got %v", actualPath)
	}
}

func Test_GetDevicePath_Device_No_Partition_Retries_Stat(t *testing.T) {
	clearTestDependencies()

	deviceName := "sdd"
	expectedDevicePath := filepath.Join("/dev", deviceName)

	osReadDir = func(_ string) ([]os.DirEntry, error) {
		entry := &fakeDirEntry{name: deviceName}
		return []os.DirEntry{entry}, nil
	}

	callNum := 0
	osStat = func(name string) (os.FileInfo, error) {
		if callNum == 0 {
			callNum += 1
			return nil, fs.ErrNotExist
		}
		if callNum == 1 {
			callNum += 1
			return nil, unix.ENXIO
		}
		return nil, nil
	}

	getDevicePath = GetDevicePath

	actualPath, err := getDevicePath(context.Background(), 0, 0, 0)
	if err != nil {
		t.Fatalf("expected to get no error, instead got %v", err)
	}
	if actualPath != expectedDevicePath {
		t.Fatalf("expected to get %v, instead got %v", expectedDevicePath, actualPath)
	}
}

func Test_GetDevicePath_Device_No_Partition_Error(t *testing.T) {
	clearTestDependencies()

	osReadDir = func(_ string) ([]os.DirEntry, error) {
		return nil, nil
	}

	osStat = func(name string) (os.FileInfo, error) {
		return nil, fmt.Errorf("should not make this call: %v", name)
	}

	getDevicePath = GetDevicePath

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	actualPath, err := getDevicePath(ctx, 0, 0, 0)
	if err == nil {
		t.Fatalf("expected to get an error, instead got %v", actualPath)
	}
}

func Test_GetDeviceFsType_Success(t *testing.T) {
	clearTestDependencies()

	devicePath := "/dev/sda"
	_tar2ext4IsDeviceExt4 = func(string) bool {
		return true
	}

	fsType, err := getDeviceFsType(devicePath)
	if err != nil {
		t.Fatal(err)
	}
	if fsType != "ext4" {
		t.Fatalf("expected to get a filesystem type of ext4, instead got %s", fsType)
	}
}

func Test_GetDeviceFsType_Error(t *testing.T) {
	clearTestDependencies()

	devicePath := "/dev/sda"
	_tar2ext4IsDeviceExt4 = func(string) bool {
		return false
	}

	fsType, err := getDeviceFsType(devicePath)
	if err == nil {
		t.Fatalf("expected to return a failure from call to getDeviceFsType, instead got %s", fsType)
	}
}
