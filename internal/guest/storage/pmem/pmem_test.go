//go:build linux
// +build linux

package pmem

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/storage/test/policy"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func clearTestDependencies() {
	osMkdirAll = nil
	osRemoveAll = nil
	unixMount = nil
	createZeroSectorLinearTarget = nil
	createVerityTarget = nil
	removeDevice = nil
	mountInternal = mount
}

func Test_Mount_Mkdir_Fails_Error(t *testing.T) {
	clearTestDependencies()

	expectedErr := errors.New("mkdir : no such file or directory")
	osMkdirAll = func(path string, perm os.FileMode) error {
		return expectedErr
	}
	err := Mount(context.Background(), 0, "", nil, nil, openDoorSecurityPolicyEnforcer())
	if errors.Cause(err) != expectedErr {
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
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	err := Mount(context.Background(), 0, target, nil, nil, openDoorSecurityPolicyEnforcer())
	if err != nil {
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
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	err := Mount(context.Background(), 0, target, nil, nil, openDoorSecurityPolicyEnforcer())
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_Calls_RemoveAll_OnMountFailure(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
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
	err := Mount(context.Background(), 0, target, nil, nil, openDoorSecurityPolicyEnforcer())
	if errors.Cause(err) != expectedErr {
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
	device := uint32(20)
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expected := fmt.Sprintf("/dev/pmem%d", device)
		if source != expected {
			t.Errorf("expected source: %s, got: %s", expected, source)
			return errors.New("unexpected source")
		}
		return nil
	}
	err := Mount(context.Background(), device, "/fake/path", nil, nil, openDoorSecurityPolicyEnforcer())
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
	expectedTarget := "/fake/path"
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if expectedTarget != target {
			t.Errorf("expected target: %s, got: %s", expectedTarget, target)
			return errors.New("unexpected target")
		}
		return nil
	}
	err := Mount(context.Background(), 0, expectedTarget, nil, nil, openDoorSecurityPolicyEnforcer())
	if err != nil {
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
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFSType := "ext4"
		if expectedFSType != fstype {
			t.Errorf("expected fstype: %s, got: %s", expectedFSType, fstype)
			return errors.New("unexpected fstype")
		}
		return nil
	}
	err := Mount(context.Background(), 0, "/fake/path", nil, nil, openDoorSecurityPolicyEnforcer())
	if err != nil {
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
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFlags := uintptr(unix.MS_RDONLY)
		if expectedFlags != flags {
			t.Errorf("expected flags: %v, got: %v", expectedFlags, flags)
			return errors.New("unexpected flags")
		}
		return nil
	}
	err := Mount(context.Background(), 0, "/fake/path", nil, nil, openDoorSecurityPolicyEnforcer())
	if err != nil {
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
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedData := "noload"
		if expectedData != data {
			t.Errorf("expected data: %s, got: %s", expectedData, data)
			return errors.New("unexpected data")
		}
		return nil
	}
	err := Mount(context.Background(), 0, "/fake/path", nil, nil, openDoorSecurityPolicyEnforcer())
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Security_Policy_Enforcement_Mount_Calls(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		return nil
	}

	enforcer := mountMonitoringSecurityPolicyEnforcer()
	err := Mount(context.Background(), 0, "/fake/path", nil, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	expectedDeviceMountCalls := 1
	if enforcer.DeviceMountCalls != expectedDeviceMountCalls {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceMountCalls, enforcer.DeviceMountCalls)
	}

	expectedDeviceUnmountCalls := 0
	if enforcer.DeviceUnmountCalls != expectedDeviceUnmountCalls {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceUnmountCalls, enforcer.DeviceUnmountCalls)
	}

	expectedOverlay := 0
	if enforcer.OverlayMountCalls != expectedOverlay {
		t.Fatalf("expected %d attempts at overlay mount enforcement, got %d", expectedOverlay, enforcer.OverlayMountCalls)
	}
}

func Test_Security_Policy_Enforcement_Unmount_Calls(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}

	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		return nil
	}

	enforcer := mountMonitoringSecurityPolicyEnforcer()
	err := Mount(context.Background(), 0, "/fake/path", nil, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	err = Unmount(context.Background(), 0, "/fake/path", nil, nil, enforcer)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	expectedDeviceMountCalls := 1
	if enforcer.DeviceMountCalls != expectedDeviceMountCalls {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceMountCalls, enforcer.DeviceMountCalls)
	}

	expectedDeviceUnmountCalls := 1
	if enforcer.DeviceUnmountCalls != expectedDeviceUnmountCalls {
		t.Fatalf("expected %d attempt at pmem mount enforcement, got %d", expectedDeviceUnmountCalls, enforcer.DeviceUnmountCalls)
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

// device mapper tests
func Test_CreateLinearTarget_And_Mount_Called_With_Correct_Parameters(t *testing.T) {
	clearTestDependencies()

	mappingInfo := &guestresource.LCOWVPMemMappingInfo{
		DeviceOffsetInBytes: 0,
		DeviceSizeInBytes:   1024,
	}
	expectedLinearName := fmt.Sprintf(linearDeviceFmt, 0, mappingInfo.DeviceOffsetInBytes, mappingInfo.DeviceSizeInBytes)
	expectedSource := "/dev/pmem0"
	expectedTarget := "/foo"
	mapperPath := fmt.Sprintf("/dev/mapper/%s", expectedLinearName)
	createZSLTCalled := false

	osMkdirAll = func(_ string, _ os.FileMode) error {
		return nil
	}

	mountInternal = func(_ context.Context, source, target string) error {
		if source != mapperPath {
			t.Errorf("expected mountInternal source %s, got %s", mapperPath, source)
		}
		if target != expectedTarget {
			t.Errorf("expected mountInternal target %s, got %s", expectedTarget, source)
		}
		return nil
	}

	createZeroSectorLinearTarget = func(_ context.Context, source, name string, mapping *guestresource.LCOWVPMemMappingInfo) (string, error) {
		createZSLTCalled = true
		if source != expectedSource {
			t.Errorf("expected createZeroSectorLinearTarget source %s, got %s", expectedSource, source)
		}
		if name != expectedLinearName {
			t.Errorf("expected createZeroSectorLinearTarget name %s, got %s", expectedLinearName, name)
		}
		return mapperPath, nil
	}

	if err := Mount(
		context.Background(),
		0,
		expectedTarget,
		mappingInfo,
		nil,
		openDoorSecurityPolicyEnforcer(),
	); err != nil {
		t.Fatalf("unexpected error during Mount: %s", err)
	}
	if !createZSLTCalled {
		t.Fatalf("createZeroSectorLinearTarget not called")
	}
}

func Test_CreateVerityTargetCalled_And_Mount_Called_With_Correct_Parameters(t *testing.T) {
	clearTestDependencies()

	verityInfo := &guestresource.DeviceVerityInfo{
		RootDigest: "hash",
	}
	expectedVerityName := fmt.Sprintf(verityDeviceFmt, 0, verityInfo.RootDigest)
	expectedSource := "/dev/pmem0"
	expectedTarget := "/foo"
	mapperPath := fmt.Sprintf("/dev/mapper/%s", expectedVerityName)
	createVerityTargetCalled := false

	mountInternal = func(_ context.Context, source, target string) error {
		if source != mapperPath {
			t.Errorf("expected mountInternal source %s, got %s", mapperPath, source)
		}
		if target != expectedTarget {
			t.Errorf("expected mountInternal target %s, got %s", expectedTarget, target)
		}
		return nil
	}
	createVerityTarget = func(_ context.Context, source, name string, verity *guestresource.DeviceVerityInfo) (string, error) {
		createVerityTargetCalled = true
		if source != expectedSource {
			t.Errorf("expected createVerityTarget source %s, got %s", expectedSource, source)
		}
		if name != expectedVerityName {
			t.Errorf("expected createVerityTarget name %s, got %s", expectedVerityName, name)
		}
		return mapperPath, nil
	}

	if err := Mount(
		context.Background(),
		0,
		expectedTarget,
		nil,
		verityInfo,
		openDoorSecurityPolicyEnforcer(),
	); err != nil {
		t.Fatalf("unexpected Mount failure: %s", err)
	}
	if !createVerityTargetCalled {
		t.Fatal("createVerityTarget not called")
	}
}

func Test_CreateLinearTarget_And_CreateVerityTargetCalled_Called_Correctly(t *testing.T) {
	clearTestDependencies()

	verityInfo := &guestresource.DeviceVerityInfo{
		RootDigest: "hash",
	}
	mapping := &guestresource.LCOWVPMemMappingInfo{
		DeviceOffsetInBytes: 0,
		DeviceSizeInBytes:   1024,
	}
	expectedLinearTarget := fmt.Sprintf(linearDeviceFmt, 0, mapping.DeviceOffsetInBytes, mapping.DeviceSizeInBytes)
	expectedVerityTarget := fmt.Sprintf(verityDeviceFmt, 0, verityInfo.RootDigest)
	expectedPMemDevice := "/dev/pmem0"
	mapperLinearPath := fmt.Sprintf("/dev/mapper/%s", expectedLinearTarget)
	mapperVerityPath := fmt.Sprintf("/dev/mapper/%s", expectedVerityTarget)
	dmLinearCalled := false
	dmVerityCalled := false
	mountCalled := false

	createZeroSectorLinearTarget = func(_ context.Context, source, name string, mapping *guestresource.LCOWVPMemMappingInfo) (string, error) {
		dmLinearCalled = true
		if source != expectedPMemDevice {
			t.Errorf("expected createZeroSectorLinearTarget source %s, got %s", expectedPMemDevice, source)
		}
		if name != expectedLinearTarget {
			t.Errorf("expected createZeroSectorLinearTarget name %s, got %s", expectedLinearTarget, name)
		}
		return mapperLinearPath, nil
	}
	createVerityTarget = func(_ context.Context, source, name string, verity *guestresource.DeviceVerityInfo) (string, error) {
		dmVerityCalled = true
		if source != mapperLinearPath {
			t.Errorf("expected createVerityTarget source %s, got %s", mapperLinearPath, source)
		}
		if name != expectedVerityTarget {
			t.Errorf("expected createVerityTarget target name %s, got %s", expectedVerityTarget, name)
		}
		return mapperVerityPath, nil
	}
	mountInternal = func(_ context.Context, source, target string) error {
		mountCalled = true
		if source != mapperVerityPath {
			t.Errorf("expected Mount source %s, got %s", mapperVerityPath, source)
		}
		return nil
	}

	if err := Mount(
		context.Background(),
		0,
		"/foo",
		mapping,
		verityInfo,
		openDoorSecurityPolicyEnforcer(),
	); err != nil {
		t.Fatalf("unexpected error during Mount call: %s", err)
	}
	if !dmLinearCalled {
		t.Fatal("expected createZeroSectorLinearTarget call")
	}
	if !dmVerityCalled {
		t.Fatal("expected createVerityTarget call")
	}
	if !mountCalled {
		t.Fatal("expected mountInternal call")
	}
}

func Test_RemoveDevice_Called_For_LinearTarget_On_MountInternalFailure(t *testing.T) {
	clearTestDependencies()

	mappingInfo := &guestresource.LCOWVPMemMappingInfo{
		DeviceOffsetInBytes: 0,
		DeviceSizeInBytes:   1024,
	}
	expectedError := errors.New("mountInternal error")
	expectedTarget := fmt.Sprintf(linearDeviceFmt, 0, mappingInfo.DeviceOffsetInBytes, mappingInfo.DeviceSizeInBytes)
	mapperPath := fmt.Sprintf("/dev/mapper/%s", expectedTarget)
	removeDeviceCalled := false

	createZeroSectorLinearTarget = func(_ context.Context, source, name string, mapping *guestresource.LCOWVPMemMappingInfo) (string, error) {
		return mapperPath, nil
	}
	mountInternal = func(_ context.Context, source, target string) error {
		return expectedError
	}
	removeDevice = func(name string) error {
		removeDeviceCalled = true
		if name != expectedTarget {
			t.Errorf("expected removeDevice linear target %s, got %s", expectedTarget, name)
		}
		return nil
	}

	if err := Mount(
		context.Background(),
		0,
		"/foo",
		mappingInfo,
		nil,
		openDoorSecurityPolicyEnforcer(),
	); err != expectedError {
		t.Fatalf("expected Mount error %s, got %s", expectedError, err)
	}
	if !removeDeviceCalled {
		t.Fatal("expected removeDevice to be callled")
	}
}

func Test_RemoveDevice_Called_For_VerityTarget_On_MountInternalFailure(t *testing.T) {
	clearTestDependencies()

	verity := &guestresource.DeviceVerityInfo{
		RootDigest: "hash",
	}
	expectedVerityTarget := fmt.Sprintf(verityDeviceFmt, 0, verity.RootDigest)
	expectedError := errors.New("mountInternal error")
	mapperPath := fmt.Sprintf("/dev/mapper/%s", expectedVerityTarget)
	removeDeviceCalled := false

	createVerityTarget = func(_ context.Context, source, name string, verity *guestresource.DeviceVerityInfo) (string, error) {
		return mapperPath, nil
	}
	mountInternal = func(_ context.Context, _, _ string) error {
		return expectedError
	}
	removeDevice = func(name string) error {
		removeDeviceCalled = true
		if name != expectedVerityTarget {
			t.Errorf("expected removeDevice verity target %s, got %s", expectedVerityTarget, name)
		}
		return nil
	}

	if err := Mount(
		context.Background(),
		0,
		"/foo",
		nil,
		verity,
		openDoorSecurityPolicyEnforcer(),
	); err != expectedError {
		t.Fatalf("expected Mount error %s, got %s", expectedError, err)
	}
	if !removeDeviceCalled {
		t.Fatal("expected removeDevice to be called")
	}
}

func Test_RemoveDevice_Called_For_Both_Targets_On_MountInternalFailure(t *testing.T) {
	clearTestDependencies()

	mapping := &guestresource.LCOWVPMemMappingInfo{
		DeviceOffsetInBytes: 0,
		DeviceSizeInBytes:   1024,
	}
	verity := &guestresource.DeviceVerityInfo{
		RootDigest: "hash",
	}
	expectedError := errors.New("mountInternal error")
	expectedLinearTarget := fmt.Sprintf(linearDeviceFmt, 0, mapping.DeviceOffsetInBytes, mapping.DeviceSizeInBytes)
	expectedVerityTarget := fmt.Sprintf(verityDeviceFmt, 0, verity.RootDigest)
	expectedPMemDevice := "/dev/pmem0"
	mapperLinearPath := fmt.Sprintf("/dev/mapper/%s", expectedLinearTarget)
	mapperVerityPath := fmt.Sprintf("/dev/mapper/%s", expectedVerityTarget)
	rmLinearCalled := false
	rmVerityCalled := false

	createZeroSectorLinearTarget = func(_ context.Context, source, name string, m *guestresource.LCOWVPMemMappingInfo) (string, error) {
		if source != expectedPMemDevice {
			t.Errorf("expected createZeroSectorLinearTarget source %s, got %s", expectedPMemDevice, source)
		}
		return mapperLinearPath, nil
	}
	createVerityTarget = func(_ context.Context, source, name string, v *guestresource.DeviceVerityInfo) (string, error) {
		if source != mapperLinearPath {
			t.Errorf("expected createVerityTarget to be called with %s, got %s", mapperLinearPath, source)
		}
		if name != expectedVerityTarget {
			t.Errorf("expected createVerityTarget target %s, got %s", expectedVerityTarget, name)
		}
		return mapperVerityPath, nil
	}
	removeDevice = func(name string) error {
		if name != expectedLinearTarget && name != expectedVerityTarget {
			t.Errorf("unexpected removeDevice target name %s", name)
		}
		if name == expectedLinearTarget {
			rmLinearCalled = true
		}
		if name == expectedVerityTarget {
			rmVerityCalled = true
		}
		return nil
	}
	mountInternal = func(_ context.Context, _, _ string) error {
		return expectedError
	}

	if err := Mount(
		context.Background(),
		0,
		"/foo",
		mapping,
		verity,
		openDoorSecurityPolicyEnforcer(),
	); err != expectedError {
		t.Fatalf("expected Mount error %s, got %s", expectedError, err)
	}
	if !rmLinearCalled {
		t.Fatal("expected removeDevice for linear target to be called")
	}
	if !rmVerityCalled {
		t.Fatal("expected removeDevice for verity target to be called")
	}
}
