//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"
	"github.com/opencontainers/runtime-spec/specs-go"
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
			opts := uvm.NewDefaultOptionsLCOW(t.Name(), t.Name())
			vm, err := uvm.CreateLCOW(ctx, opts)
			if err != nil {
				t.Fatalf("failed to create LCOW UVM: %s", err)
			}
			if err := vm.Start(ctx); err != nil {
				t.Fatalf("failed to start LCOW UVM: %s", err)
			}
			t.Cleanup(func() {
				if err := vm.Close(); err != nil {
					t.Log(err)
				}
			})
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
