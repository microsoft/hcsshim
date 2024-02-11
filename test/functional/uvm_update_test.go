//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"strings"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/internal/protocol/guestrequest"
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/Microsoft/hcsshim/pkg/ctrdtaskapi"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestLCOW_Update_Resources(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM)
	require.Build(t, osversion.RS5)

	ctx := util.Context(context.Background(), t)

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
			vm, cleanup := testuvm.CreateLCOW(ctx, t, defaultLCOWOptions(ctx, t))
			testuvm.Start(ctx, t, vm)
			defer cleanup(ctx)
			if err := vm.Update(ctx, config.resource, nil); err != nil {
				if config.valid {
					if strings.Contains(err.Error(), "invalid resource") {
						t.Fatalf("failed to update LCOW UVM constraints: %s", err)
					} else {
						t.Logf("ignored error: %s", err)
					}
				}
			} else {
				if !config.valid {
					t.Fatal("expected error updating LCOW UVM constraints")
				}
			}
		})
	}
}
