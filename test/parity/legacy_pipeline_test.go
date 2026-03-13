//go:build windows && functional

package parity

import (
	"context"
	"fmt"

	"github.com/opencontainers/runtime-spec/specs-go"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/uvm"
)

// buildLegacyDocument runs the full old shim pipeline:
//
//  1. oci.UpdateSpecFromOptions — merge shim options into spec annotations
//  2. oci.ProcessAnnotations — expand annotation groups
//  3. oci.SpecToUVMCreateOpts — spec → *OptionsLCOW
//  4. uvm.MakeLCOWDocument — OptionsLCOW → HCS document
func buildLegacyDocument(ctx context.Context, spec specs.Spec, shimOpts *runhcsopts.Options, bundle string) (*hcsschema.ComputeSystem, *uvm.OptionsLCOW, error) {
	spec = oci.UpdateSpecFromOptions(spec, shimOpts)

	if err := oci.ProcessAnnotations(ctx, spec.Annotations); err != nil {
		return nil, nil, fmt.Errorf("ProcessAnnotations: %w", err)
	}

	rawOpts, err := oci.SpecToUVMCreateOpts(ctx, &spec, "test-parity@vm", "test-owner")
	if err != nil {
		return nil, nil, fmt.Errorf("SpecToUVMCreateOpts: %w", err)
	}

	opts := rawOpts.(*uvm.OptionsLCOW)
	opts.BundleDirectory = bundle

	doc, err := uvm.MakeLCOWDocument(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("MakeLCOWDocument: %w", err)
	}

	return doc, opts, nil
}
