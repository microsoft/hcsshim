//go:build windows && functional

package functional

import (
	"context"
	"fmt"
	"testing"

	ctrdoci "github.com/containerd/containerd/oci"

	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/securitypolicy"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	testimages "github.com/Microsoft/hcsshim/test/pkg/images"
	policytest "github.com/Microsoft/hcsshim/test/pkg/securitypolicy"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func setupScratchTemplate(ctx context.Context, tb testing.TB) string {
	tb.Helper()
	opts := defaultLCOWOptions(ctx, tb)
	vm, err := uvm.CreateLCOW(ctx, opts)
	if err != nil {
		tb.Fatalf("failed to create scratch formatting uVM: %s", err)
	}
	if err := vm.Start(ctx); err != nil {
		tb.Fatalf("failed to start scratch formatting uVM: %s", err)
	}
	defer testuvm.Close(ctx, tb, vm)
	scratch, _ := testlayers.ScratchSpace(ctx, tb, vm, "", "", "")
	return scratch
}

func TestGetProperties_WithPolicy(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM, featureLCOWIntegrity)

	ctx := util.Context(namespacedContext(context.Background()), t)
	scratchPath := setupScratchTemplate(ctx, t)

	ls := linuxImageLayers(ctx, t)
	for _, allowProperties := range []bool{true, false} {
		t.Run(fmt.Sprintf("AllowPropertiesAccess_%t", allowProperties), func(t *testing.T) {
			opts := defaultLCOWOptions(ctx, t)
			policy := policytest.PolicyFromImageWithOpts(
				t,
				testimages.ImageLinuxAlpineLatest,
				"rego",
				[]securitypolicy.ContainerConfigOpt{
					securitypolicy.WithCommand([]string{"/bin/sh", "-c", testoci.TailNullArgs}),
				},
				[]securitypolicy.PolicyConfigOpt{
					securitypolicy.WithAllowPropertiesAccess(allowProperties),
					securitypolicy.WithAllowUnencryptedScratch(true),
				},
			)
			opts.SecurityPolicyEnforcer = "rego"
			opts.SecurityPolicy = policy

			cleanName := util.CleanName(t.Name())
			vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)
			spec := testoci.CreateLinuxSpec(
				ctx,
				t,
				cleanName,
				testoci.DefaultLinuxSpecOpts(
					"",
					ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
					testoci.WithWindowsLayerFolders(append(ls, scratchPath)),
				)...,
			)

			c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cleanName, hcsOwner)
			t.Cleanup(cleanup)

			init := testcontainer.Start(ctx, t, c, nil)
			t.Cleanup(func() {
				testcontainer.Kill(ctx, t, c)
				testcontainer.Wait(ctx, t, c)
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

			testcmd.Kill(ctx, t, init)
			testcmd.WaitExitCode(ctx, t, init, testcmd.ForcedKilledExitCode)
		})
	}
}
