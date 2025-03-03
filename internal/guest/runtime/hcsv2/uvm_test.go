//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/guest/storage/scsi"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

const (
	testContainerID      = "test-container"
	testDevicePath       = "/dev/sde"
	testMountPathOnGuest = "/mount/path"
)

var testMountPathForContainerDeviceOnGuest = fmt.Sprintf(guestpath.LCOWSCSIMountPrefixFmt, 3)

func Test_ModifyHostSettings_VirtualDisk(t *testing.T) {
	tests := []struct {
		name           string
		requestType    guestrequest.RequestType
		mountPath      string
		containerMount bool
		readonly       bool
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "ValidMountOnGuest_Add_RW",
			requestType:    guestrequest.RequestTypeAdd,
			mountPath:      testMountPathOnGuest,
			containerMount: false,
			readonly:       false,
			expectError:    false,
			errorMessage:   "",
		},
		{
			name:           "ValidMountOnGuest_Add_RO",
			requestType:    guestrequest.RequestTypeAdd,
			mountPath:      testMountPathOnGuest,
			containerMount: false,
			readonly:       true,
			expectError:    false,
			errorMessage:   "",
		},
		{
			name:           "ValidMountOnGuest_ContainerDevice_Add_RW",
			requestType:    guestrequest.RequestTypeAdd,
			mountPath:      testMountPathForContainerDeviceOnGuest,
			containerMount: true,
			readonly:       false,
			expectError:    false,
			errorMessage:   "",
		},
		{
			name:           "ValidMountOnGuest_ContainerDevice_Add_RO",
			requestType:    guestrequest.RequestTypeAdd,
			mountPath:      testMountPathForContainerDeviceOnGuest,
			containerMount: true,
			readonly:       true,
			expectError:    false,
			errorMessage:   "",
		},
		{
			name:         "ValidMountOnGuest_Remove",
			requestType:  guestrequest.RequestTypeRemove,
			mountPath:    testMountPathForContainerDeviceOnGuest,
			expectError:  false,
			errorMessage: "",
		},
		{
			name:           "InvalidMountOnGuest_ContainerDevice",
			requestType:    guestrequest.RequestTypeAdd,
			mountPath:      "/invalid/mount/path",
			containerMount: true,
			expectError:    true,
			errorMessage:   "invalid mount path inside guest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(nil, nil, &securitypolicy.OpenDoorSecurityPolicyEnforcer{}, os.Stdout)
			ctx := context.Background()

			// Mock functions
			scsiActualControllerNumberFn = func(ctx context.Context, controller uint8) (uint8, error) {
				return controller, nil
			}
			scsiGetDevicePathFn = func(ctx context.Context, controller uint8, lun uint8, partition uint64) (string, error) {
				return testDevicePath, nil
			}
			scsiMountFn = func(ctx context.Context, controller uint8, lun uint8, partition uint64, mountPath string, readOnly bool, options []string, config *scsi.Config) error {
				return nil
			}
			scsiUnmountFn = func(ctx context.Context, controller uint8, lun uint8, partition uint64, mountPath string, config *scsi.Config) error {
				return nil
			}
			// Restore the original functions after the test.
			defer func() {
				scsiActualControllerNumberFn = scsi.ActualControllerNumber
				scsiGetDevicePathFn = scsi.GetDevicePath
				scsiMountFn = scsi.Mount
				scsiUnmountFn = scsi.Unmount
			}()

			// Create the modification request.
			req := &guestrequest.ModificationRequest{
				ResourceType: guestresource.ResourceTypeMappedVirtualDisk,
				RequestType:  guestrequest.RequestTypeAdd,
				Settings: &guestresource.LCOWMappedVirtualDisk{
					ReadOnly:       tt.readonly,
					ContainerMount: tt.containerMount,
					Controller:     0,
					Lun:            0,
					Partition:      1,
					Encrypted:      false,
					MountPath:      tt.mountPath,
				},
			}

			// Run the test.
			err := h.modifyHostSettings(ctx, testContainerID, req)
			if err != nil {
				// If an error was expected then validate the error message.
				if tt.expectError && !strings.Contains(err.Error(), tt.errorMessage) {
					t.Fatalf("expected error %s, got: %v", tt.errorMessage, err)
				}

				// If the error was not expected, then fail the test.
				if !tt.expectError {
					t.Fatalf("expected no error, got: %v", err)
				}
			}

			if err == nil {
				if tt.expectError {
					t.Fatalf("expected error %s but got nil", tt.errorMessage)
				}
			}
		})
	}
}
