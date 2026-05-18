//go:build windows && lcow

package linuxcontainer

import (
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/internal/controller/device/vpci"
	"github.com/Microsoft/hcsshim/internal/controller/linuxcontainer/mocks"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/opencontainers/runtime-spec/specs-go"
	"go.uber.org/mock/gomock"
)

// newTestControllerAndSpec creates a Controller wired to a fresh vPCIController
// mock alongside a minimal OCI spec populated with the provided Windows devices.
func newTestControllerAndSpec(t *testing.T, devices ...specs.WindowsDevice) (*Controller, *specs.Spec, *mocks.MockvPCIController) {
	t.Helper()
	ctrl := gomock.NewController(t)
	vpciCtrl := mocks.NewMockvPCIController(ctrl)
	return &Controller{vpci: vpciCtrl}, &specs.Spec{
		Windows: &specs.Windows{
			Devices: devices,
		},
	}, vpciCtrl
}

var (
	errReserve = errors.New("reserve failed")
	errAddToVM = errors.New("add to vm failed")
)

// TestAllocateDevices_NoDevices verifies that allocateDevices succeeds without
// any vPCI calls when the spec contains no Windows devices.
func TestAllocateDevices_NoDevices(t *testing.T) {
	t.Parallel()
	c, spec, _ := newTestControllerAndSpec(t)

	if err := c.allocateDevices(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.devices) != 0 {
		t.Errorf("expected 0 tracked devices, got %d", len(c.devices))
	}
}

// TestAllocateDevices_InvalidDeviceType verifies that allocateDevices returns an
// error for unsupported device types, regardless of position in the device list.
func TestAllocateDevices_InvalidDeviceType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		devices []specs.WindowsDevice
	}{
		{
			name: "single-invalid",
			devices: []specs.WindowsDevice{
				{ID: "PCI\\VEN_1234&DEV_5678\\0", IDType: "unsupported-type"},
			},
		},
		{
			name: "invalid-before-valid",
			devices: []specs.WindowsDevice{
				{ID: "PCI\\VEN_AAAA&DEV_1111\\0", IDType: "bad-type"},
				{ID: "PCI\\VEN_BBBB&DEV_2222\\0", IDType: vpci.DeviceIDType},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, spec, _ := newTestControllerAndSpec(t, tt.devices...)

			// No Reserve or AddToVM calls expected.

			if err := c.allocateDevices(t.Context(), spec); err == nil {
				t.Fatal("expected error for unsupported device type")
			}
			if len(c.devices) != 0 {
				t.Errorf("expected 0 tracked devices, got %d", len(c.devices))
			}
		})
	}
}

// TestAllocateDevices_SingleDevice verifies the Reserve → AddToVM flow for each
// supported device type, including VF index parsing and spec ID rewrite.
func TestAllocateDevices_SingleDevice(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		deviceID  string
		idType    string
		expectPCI string
		expectVF  uint16
	}{
		{
			name:      "vpci-instance-id",
			deviceID:  "PCI\\VEN_1234&DEV_5678\\0",
			idType:    vpci.DeviceIDType,
			expectPCI: "PCI\\VEN_1234&DEV_5678",
			expectVF:  0,
		},
		{
			name:      "vpci-legacy-with-vf-index",
			deviceID:  "PCI\\VEN_1234&DEV_5678\\0/3",
			idType:    vpci.DeviceIDTypeLegacy,
			expectPCI: "PCI\\VEN_1234&DEV_5678\\0",
			expectVF:  3,
		},
		{
			name:      "gpu",
			deviceID:  "PCI\\VEN_ABCD&DEV_9876\\0",
			idType:    vpci.GpuDeviceIDType,
			expectPCI: "PCI\\VEN_ABCD&DEV_9876",
			expectVF:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, spec, vpciCtrl := newTestControllerAndSpec(t, specs.WindowsDevice{
				ID:     tt.deviceID,
				IDType: tt.idType,
			})

			testGUID, _ := guid.NewV4()

			vpciCtrl.EXPECT().
				Reserve(gomock.Any(), vpci.Device{
					DeviceInstanceID:     tt.expectPCI,
					VirtualFunctionIndex: tt.expectVF,
				}).
				Return(testGUID, nil)
			vpciCtrl.EXPECT().
				AddToVM(gomock.Any(), testGUID).
				Return(nil)

			if err := c.allocateDevices(t.Context(), spec); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the spec entry was rewritten to the VMBus GUID.
			if got := spec.Windows.Devices[0].ID; got != testGUID.String() {
				t.Errorf("spec device ID = %q, want %q", got, testGUID.String())
			}

			// Verify the GUID was tracked.
			if len(c.devices) != 1 || c.devices[0] != testGUID {
				t.Errorf("tracked devices = %v, want [%v]", c.devices, testGUID)
			}
		})
	}
}

// TestAllocateDevices_SingleDeviceFailure verifies that Reserve and AddToVM
// failures are propagated and no device is tracked.
func TestAllocateDevices_SingleDeviceFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		reserveErr  error
		addToVMErr  error
		wantWrapped error
	}{
		{
			name:        "reserve-fails",
			reserveErr:  errReserve,
			wantWrapped: errReserve,
		},
		{
			name:        "add-to-vm-fails",
			addToVMErr:  errAddToVM,
			wantWrapped: errAddToVM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, spec, vpciCtrl := newTestControllerAndSpec(t, specs.WindowsDevice{
				ID:     "PCI\\VEN_1234&DEV_5678\\0",
				IDType: vpci.DeviceIDType,
			})

			testGUID, _ := guid.NewV4()
			vpciCtrl.EXPECT().
				Reserve(gomock.Any(), gomock.Any()).
				Return(testGUID, tt.reserveErr)

			// AddToVM is only called when Reserve succeeds.
			if tt.reserveErr == nil {
				vpciCtrl.EXPECT().
					AddToVM(gomock.Any(), testGUID).
					Return(tt.addToVMErr)
			}

			err := c.allocateDevices(t.Context(), spec)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.wantWrapped) {
				t.Errorf("error = %v, want wrapping %v", err, tt.wantWrapped)
			}
			// Reserve appends to c.devices BEFORE AddToVM is invoked, so a
			// Reserve failure leaves nothing tracked, while an AddToVM
			// failure leaves the device tracked for unwind.
			wantTracked := 0
			if tt.reserveErr == nil {
				wantTracked = 1
			}
			if len(c.devices) != wantTracked {
				t.Errorf("expected %d tracked devices, got %d", wantTracked, len(c.devices))
			}
		})
	}
}

// TestAllocateDevices_MultipleDevices verifies that allocateDevices correctly
// handles multiple devices, reserving and adding each one independently.
func TestAllocateDevices_MultipleDevices(t *testing.T) {
	t.Parallel()
	c, spec, vpciCtrl := newTestControllerAndSpec(t,
		specs.WindowsDevice{ID: "PCI\\VEN_AAAA&DEV_1111\\0", IDType: vpci.DeviceIDType},
		specs.WindowsDevice{ID: "PCI\\VEN_BBBB&DEV_2222\\0", IDType: vpci.GpuDeviceIDType},
	)

	guidA, _ := guid.NewV4()
	guidB, _ := guid.NewV4()

	vpciCtrl.EXPECT().
		Reserve(gomock.Any(), vpci.Device{
			DeviceInstanceID:     "PCI\\VEN_AAAA&DEV_1111",
			VirtualFunctionIndex: 0,
		}).
		Return(guidA, nil)
	vpciCtrl.EXPECT().
		AddToVM(gomock.Any(), guidA).
		Return(nil)

	vpciCtrl.EXPECT().
		Reserve(gomock.Any(), vpci.Device{
			DeviceInstanceID:     "PCI\\VEN_BBBB&DEV_2222",
			VirtualFunctionIndex: 0,
		}).
		Return(guidB, nil)
	vpciCtrl.EXPECT().
		AddToVM(gomock.Any(), guidB).
		Return(nil)

	if err := c.allocateDevices(t.Context(), spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.devices) != 2 {
		t.Fatalf("expected 2 tracked devices, got %d", len(c.devices))
	}
	if c.devices[0] != guidA || c.devices[1] != guidB {
		t.Errorf("tracked GUIDs = %v, %v; want %v, %v", c.devices[0], c.devices[1], guidA, guidB)
	}
	if spec.Windows.Devices[0].ID != guidA.String() {
		t.Errorf("first device ID = %q, want %q", spec.Windows.Devices[0].ID, guidA.String())
	}
	if spec.Windows.Devices[1].ID != guidB.String() {
		t.Errorf("second device ID = %q, want %q", spec.Windows.Devices[1].ID, guidB.String())
	}
}

// TestAllocateDevices_MultipleDevicesPartialFailure verifies that when the
// second device fails (at Reserve or AddToVM), the first device is tracked
// but the overall call returns the expected error.
func TestAllocateDevices_MultipleDevicesPartialFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		reserveErr  error
		addToVMErr  error
		wantWrapped error
	}{
		{
			name:        "second-reserve-fails",
			reserveErr:  errReserve,
			wantWrapped: errReserve,
		},
		{
			name:        "second-add-to-vm-fails",
			addToVMErr:  errAddToVM,
			wantWrapped: errAddToVM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, spec, vpciCtrl := newTestControllerAndSpec(t,
				specs.WindowsDevice{ID: "PCI\\VEN_AAAA&DEV_1111\\0", IDType: vpci.DeviceIDType},
				specs.WindowsDevice{ID: "PCI\\VEN_BBBB&DEV_2222\\0", IDType: vpci.DeviceIDType},
			)

			guidA, _ := guid.NewV4()
			guidB, _ := guid.NewV4()

			// First device always succeeds.
			vpciCtrl.EXPECT().
				Reserve(gomock.Any(), vpci.Device{
					DeviceInstanceID:     "PCI\\VEN_AAAA&DEV_1111",
					VirtualFunctionIndex: 0,
				}).
				Return(guidA, nil)
			vpciCtrl.EXPECT().
				AddToVM(gomock.Any(), guidA).
				Return(nil)

			// Second device fails at the configured step.
			vpciCtrl.EXPECT().
				Reserve(gomock.Any(), vpci.Device{
					DeviceInstanceID:     "PCI\\VEN_BBBB&DEV_2222",
					VirtualFunctionIndex: 0,
				}).
				Return(guidB, tt.reserveErr)

			// AddToVM for the second device is only called when its Reserve succeeds.
			if tt.reserveErr == nil {
				vpciCtrl.EXPECT().
					AddToVM(gomock.Any(), guidB).
					Return(tt.addToVMErr)
			}

			err := c.allocateDevices(t.Context(), spec)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.wantWrapped) {
				t.Errorf("error = %v, want wrapping %v", err, tt.wantWrapped)
			}

			// First device was already allocated before the second failed.
			// When the second Reserve fails, only the first device is
			// tracked. When the second AddToVM fails, the second device has
			// also been appended to c.devices (append happens before
			// AddToVM), leaving 2 tracked for unwind.
			wantTracked := 1
			if tt.reserveErr == nil {
				wantTracked = 2
			}
			if len(c.devices) != wantTracked {
				t.Errorf("expected %d tracked device(s) after partial failure, got %d", wantTracked, len(c.devices))
			}
		})
	}
}
