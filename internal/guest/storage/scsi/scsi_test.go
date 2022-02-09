//go:build linux
// +build linux

package scsi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/storage/test/policy"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func clearTestDependencies() {
	osMkdirAll = nil
	osRemoveAll = nil
	unixMount = nil
	controllerLunToName = nil
	createVerityTarget = nil
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
		openDoorSecurityPolicyEnforcer(),
	); err != expectedErr {
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
	); err != expectedErr {
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
	err := Mount(context.Background(), 0, 0, "/fake/path", false, false, nil, nil, openDoorSecurityPolicyEnforcer())
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
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
		if "" != data {
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
	); err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Read_Only_Security_Policy_Enforcement_Mount_Calls(t *testing.T) {
	clearTestDependencies()

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

	enforcer := mountMonitoringSecurityPolicyEnforcer()
	err := Mount(context.Background(), 0, 0, target, true, false, nil, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	expectedDeviceMounts := 1
	if enforcer.DeviceMountCalls != expectedDeviceMounts {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceMounts, enforcer.DeviceMountCalls)
	}

	expectedDeviceUnmounts := 0
	if enforcer.DeviceUnmountCalls != expectedDeviceUnmounts {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceUnmounts, enforcer.DeviceUnmountCalls)
	}

	expectedOverlay := 0
	if enforcer.OverlayMountCalls != expectedOverlay {
		t.Fatalf("expected %d attempts at overlay mount enforcement, got %d", expectedOverlay, enforcer.OverlayMountCalls)
	}
}

func Test_Read_Write_Security_Policy_Enforcement_Mount_Calls(t *testing.T) {
	clearTestDependencies()

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

	enforcer := mountMonitoringSecurityPolicyEnforcer()
	err := Mount(context.Background(), 0, 0, target, false, false, nil, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	expectedDeviceMounts := 0
	if enforcer.DeviceMountCalls != expectedDeviceMounts {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceMounts, enforcer.DeviceMountCalls)
	}

	expectedDeviceUnmounts := 0
	if enforcer.DeviceUnmountCalls != expectedDeviceUnmounts {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceUnmounts, enforcer.DeviceUnmountCalls)
	}

	expectedOverlay := 0
	if enforcer.OverlayMountCalls != expectedOverlay {
		t.Fatalf("expected %d attempts at overlay mount enforcement, got %d", expectedOverlay, enforcer.OverlayMountCalls)
	}
}

func Test_Security_Policy_Enforcement_Unmount_Calls(t *testing.T) {
	clearTestDependencies()

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

	enforcer := mountMonitoringSecurityPolicyEnforcer()
	err := Mount(context.Background(), 0, 0, target, true, false, nil, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	err = Unmount(context.Background(), 0, 0, target, false, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	expectedDeviceMounts := 1
	if enforcer.DeviceMountCalls != expectedDeviceMounts {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceMounts, enforcer.DeviceMountCalls)
	}

	expectedDeviceUnmounts := 1
	if enforcer.DeviceUnmountCalls != expectedDeviceUnmounts {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceMounts, enforcer.DeviceUnmountCalls)
	}

	expectedOverlay := 0
	if enforcer.OverlayMountCalls != expectedOverlay {
		t.Fatalf("expected %d attempts at overlay mount enforcement, got %d", expectedOverlay, enforcer.OverlayMountCalls)
	}
}

func openDoorSecurityPolicyEnforcer() securitypolicy.SecurityPolicyEnforcer {
	return &securitypolicy.OpenDoorSecurityPolicyEnforcer{}
}

func mountMonitoringSecurityPolicyEnforcer() *policy.MountMonitoringSecurityPolicyEnforcer {
	return &policy.MountMonitoringSecurityPolicyEnforcer{}
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
		openDoorSecurityPolicyEnforcer(),
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
		openDoorSecurityPolicyEnforcer(),
	); err != expectedError {
		t.Fatalf("expected Mount error %s, got %s", expectedError, err)
	}
	if !removeDeviceCalled {
		t.Fatal("expected removeDevice to be called")
	}
}
