//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"

	"github.com/Microsoft/hcsshim/test/internal/uvm"
)

func Test_LCOW_Update_Resources(t *testing.T) {
	for _, config := range []struct {
		name     string
		resource interface{}
		valid    bool
	}{
		{
			name:     "Valid_LinuxResources",
			resource: &specs.LinuxResources{},
			valid:    true,
		},
		{
			name:     "Valid_WindowsResources",
			resource: &specs.WindowsResources{},
			valid:    true,
		},
		{
			name:     "Valid_PolicyFragment",
			resource: &ctrdtaskapi.PolicyFragment{},
			valid:    true,
		},
		{
			name:     "Invalid_Mount",
			resource: &specs.Mount{},
			valid:    false,
		},
		{
			name:     "Invalid_LCOWNetwork",
			resource: &guestrequest.NetworkModifyRequest{},
			valid:    false,
		},
	} {
		t.Run(config.name, func(t *testing.T) {
			ctx := context.Background()
			vm := uvm.CreateLCOW(ctx, t, defaultLCOWOptions(t))
			cleanup := uvm.Start(ctx, t, vm)
			defer cleanup()
			if err := vm.Update(ctx, config.resource, nil); err != nil {
				if config.valid {
					t.Fatalf("failed to update LCOW UVM constraints: %s", err)
				}
			} else {
				if !config.valid {
					t.Fatal("expected error updating LCOW UVM constraints")
				}
			}
		})
	}
}
