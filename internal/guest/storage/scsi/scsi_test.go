//go:build linux
// +build linux

package scsi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"golang.org/x/sys/unix"
)

func clearTestDependencies() {
	osMkdirAll = nil
	osRemoveAll = nil
	unixMount = nil
	controllerLunToName = nil
	createVerityTarget = nil
	encryptDevice = nil
	cleanupCryptDevice = nil
	storageUnmountPath = nil
}

func Test_Mount_Mkdir_Fails_Error(t *testing.T) {
	clearTestDependencies()

	expectedErr := errors.New("mkdir : no such file or directory")
	osMkdirAll = func(path string, perm os.FileMode) error {
		return expectedErr
	}

	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}

	if err := Mount(
		context.Background(),
		0,
		0,
		"",
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}

	if err := Mount(
		context.Background(),
		0,
		0,
		target,
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}

	if err := Mount(
		context.Background(),
		0,
		0,
		target,
		false,
		false,
		nil,
		nil,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_ControllerLunToName_Valid_Controller(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedController := uint8(2)
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		expectedController,
		0,
		"/fake/path",
		false,
		false,
		nil,
		nil,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_ControllerLunToName_Valid_Lun(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedLun := uint8(2)
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		expectedLun,
		"/fake/path",
		false,
		false,
		nil,
		nil,
	); err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_Calls_RemoveAll_OnMountFailure(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		target,
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return expectedSource, nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if expectedSource != source {
			t.Errorf("expected source: %s, got: %s", expectedSource, source)
			return errors.New("unexpected source")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", false, false, nil, nil)
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		expectedTarget,
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		true,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if data != "" {
			t.Errorf("expected empty data, got: %s", data)
			return errors.New("unexpected data")
		}
		return nil
	}

	if err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		false,
		false,
		nil,
		nil,
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
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		true,
		false,
		nil,
		nil,
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

// dm-verity tests

func Test_CreateVerityTarget_And_Mount_Called_With_Correct_Parameters(t *testing.T) {
	clearTestDependencies()

	expectedVerityName := fmt.Sprintf(verityDeviceFmt, 0, 0, "hash")
	expectedSource := "/dev/sdb"
	expectedMapperPath := fmt.Sprintf("/dev/mapper/%s", expectedVerityName)
	expectedTarget := "/foo"
	createVerityTargetCalled := false

	controllerLunToName = func(_ context.Context, _, _ uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		expectedTarget,
		true,
		false,
		nil,
		vInfo,
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
	expectedVerityName := fmt.Sprintf(verityDeviceFmt, 0, 0, "hash")
	removeDeviceCalled := false

	controllerLunToName = func(_ context.Context, _, _ uint8) (string, error) {
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

	if err := Mount(
		context.Background(),
		0,
		0,
		"/foo",
		true,
		false,
		nil,
		verityInfo,
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
	controllerLunToName = func(context.Context, uint8, uint8) (string, error) {
		return "", nil
	}
	unixMount = func(string, string, string, uintptr, string) error {
		return nil
	}
	encryptDeviceCalled := false
	encryptDevice = func(_ context.Context, source string, devName string) (string, error) {
		expectedCryptTarget := fmt.Sprintf(cryptDeviceFmt, 0, 0)
		if devName != expectedCryptTarget {
			t.Fatalf("expected crypt device %q got %q", expectedCryptTarget, devName)
		}
		encryptDeviceCalled = true
		return "", nil
	}
	if err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		false,
		true,
		nil,
		nil,
	); err != nil {
		t.Fatalf("expected nil error, got: %s", err)
	}
	if !encryptDeviceCalled {
		t.Fatal("expected encryptDevice to be called")
	}
}

func Test_Mount_RemoveAllCalled_When_EncryptDevice_Fails(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(string, os.FileMode) error {
		return nil
	}
	controllerLunToName = func(context.Context, uint8, uint8) (string, error) {
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

	err := Mount(
		context.Background(),
		0,
		0,
		"/fake/path",
		false,
		true,
		nil,
		nil,
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
	cleanupCryptDevice = func(devName string) error {
		expectedDevName := fmt.Sprintf(cryptDeviceFmt, 0, 0)
		if devName != expectedDevName {
			t.Fatalf("expected crypt target %q, got %q", expectedDevName, devName)
		}
		cleanupCryptDeviceCalled = true
		return nil
	}

	if err := Unmount(context.Background(), 0, 0, "/fake/path", true, nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !cleanupCryptDeviceCalled {
		t.Fatal("cleanupCryptDevice not called")
	}
}
