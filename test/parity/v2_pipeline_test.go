//go:build windows && functional

package parity

import (
	"context"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	lcowbuilder "github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	vm "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"
)

// buildV2Document runs the v2 shim pipeline:
//
//	vm.Spec + runhcsopts.Options → lcow.BuildSandboxConfig → HCS document
func buildV2Document(ctx context.Context, shimOpts *runhcsopts.Options, spec *vm.Spec, bundle string) (*hcsschema.ComputeSystem, *lcowbuilder.SandboxOptions, error) {
	return lcowbuilder.BuildSandboxConfig(ctx, "test-owner", bundle, shimOpts, spec)
}
