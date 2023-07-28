//go:build windows && functional

package functional

import (
	"context"
	"fmt"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"
	"github.com/Microsoft/hcsshim/test/internal/cmd"
	"github.com/Microsoft/hcsshim/test/internal/container"
	"github.com/Microsoft/hcsshim/test/internal/layers"
	"github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/images"
	policytest "github.com/Microsoft/hcsshim/test/pkg/securitypolicy"
	uvmtest "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func setupScratchTemplate(ctx context.Context, tb testing.TB) string {
	tb.Helper()
	opts := defaultLCOWOptions(tb)
	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		tb.Fatalf("failed to create scratch formatting uVM: %s", err)
	}
	if err := vm.Start(ctx); err != nil {
		tb.Fatalf("failed to start scratch formatting uVM: %s", err)
	}
	defer vm.Close()
	scratch, _ := layers.ScratchSpace(ctx, tb, vm, "", "", "")
	return scratch
}

func Test_GetProperties_WithPolicy(t *testing.T) {
	requireFeatures(t, featureLCOWIntegrity)

	ctx := namespacedContext()
	scratchPath := setupScratchTemplate(ctx, t)

	ls := linuxImageLayers(ctx, t)
	for _, allowProperties := range []bool{true, false} {
		t.Run(fmt.Sprintf("AllowPropertiesAccess_%t", allowProperties), func(t *testing.T) {
			opts := defaultLCOWOptions(t)
			policy := policytest.PolicyFromImageWithOpts(
				t,
				images.ImageLinuxAlpineLatest,
				"rego",
				[]securitypolicy.ContainerConfigOpt{
					securitypolicy.WithCommand([]string{"/bin/sh", "-c", oci.TailNullArgs}),
				},
				[]securitypolicy.PolicyConfigOpt{
					securitypolicy.WithAllowPropertiesAccess(allowProperties),
					securitypolicy.WithAllowUnencryptedScratch(true),
				},
			)
			opts.SecurityPolicyEnforcer = "rego"
			opts.SecurityPolicy = policy

			cleanName := util.CleanName(t.Name())
			vm := uvmtest.CreateAndStartLCOWFromOpts(ctx, t, opts)
			spec := oci.CreateLinuxSpec(
				ctx,
				t,
				cleanName,
				oci.DefaultLinuxSpecOpts(
					"",
					ctrdoci.WithProcessArgs("/bin/sh", "-c", oci.TailNullArgs),
					oci.WithWindowsLayerFolders(append(ls, scratchPath)),
				)...,
			)

			c, _, cleanup := container.Create(ctx, t, vm, spec, cleanName, hcsOwner)
			t.Cleanup(cleanup)

			init := container.Start(ctx, t, c, nil)
			t.Cleanup(func() {
				container.Kill(ctx, t, c)
				container.Wait(ctx, t, c)
			})

			_, err := c.Properties(ctx)
			if err != nil {
				if allowProperties {
					t.Fatalf("get properties should have been allowed: %s", err)
				}
				if !(policytest.AssertErrorContains(t, err, "deny") &&
					policytest.AssertErrorContains(t, err, "get_properties")) {
					t.Fatalf("get properties denial error, got: %s", err)
				}
			} else {
				if !allowProperties {
					t.Fatal("get properties should have failed")
				}
			}

			cmd.Kill(ctx, t, init)
			cmd.WaitExitCode(ctx, t, init, cmd.ForcedKilledExitCode)
		})
	}
}
