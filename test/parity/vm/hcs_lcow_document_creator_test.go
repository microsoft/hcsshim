//go:build windows && lcow

package vmparity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	lcowbuilder "github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/oci"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/internal/vm/vmutils"
	"github.com/Microsoft/hcsshim/osversion"
	vm "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"
)

// buildLegacyLCOWDocument creates the HCS document for an LCOW VM using the
// legacy shim pipeline. It runs the same sequence as createInternal → createPod
// → CreateLCOW: annotation processing, spec conversion, option verification,
// and document generation.
func buildLegacyLCOWDocument(
	ctx context.Context,
	spec specs.Spec,
	shimOpts *runhcsopts.Options,
	bundle string,
) (*hcsschema.ComputeSystem, *uvm.OptionsLCOW, error) {
	// Step 1: Merge shim options into the OCI spec annotations.
	spec = oci.UpdateSpecFromOptions(spec, shimOpts)

	// Step 2: Expand annotation groups (e.g., security toggles).
	if err := oci.ProcessAnnotations(ctx, spec.Annotations); err != nil {
		return nil, nil, fmt.Errorf("failed to expand OCI annotations: %w", err)
	}

	// Step 3: Convert OCI spec + annotations into OptionsLCOW.
	rawOpts, err := oci.SpecToUVMCreateOpts(ctx, &spec, "test-parity@vm", "test-owner")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert OCI spec to UVM create options: %w", err)
	}
	opts := rawOpts.(*uvm.OptionsLCOW)
	opts.BundleDirectory = bundle

	// Step 4: Verify options constraints (same as CreateLCOW).
	if err := uvm.VerifyOptions(ctx, opts); err != nil {
		return nil, nil, fmt.Errorf("option verification failed: %w", err)
	}

	// Step 5: Build the temporary UtilityVM with fields that MakeLCOWDoc reads.
	scsiCount := opts.SCSIControllerCount
	if osversion.Build() >= osversion.RS5 && opts.VPMemDeviceCount == 0 {
		scsiCount = 4
	}
	tempUVM := uvm.NewUtilityVMForDoc(
		opts.ID, opts.Owner,
		scsiCount, opts.VPMemDeviceCount, opts.VPMemSizeBytes,
		!opts.VPMemNoMultiMapping,
	)

	// Step 6: Generate the HCS document.
	doc, err := uvm.MakeLCOWDoc(ctx, opts, tempUVM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate legacy LCOW HCS document: %w", err)
	}

	return doc, opts, nil
}

// buildV2LCOWDocument creates the HCS document and sandbox options from the
// provided VM spec and runhcs options using the v2 modular builder.
// The returned document can be used to create a VM directly via HCS.
func buildV2LCOWDocument(
	ctx context.Context,
	shimOpts *runhcsopts.Options,
	spec *vm.Spec,
	bundle string,
) (*hcsschema.ComputeSystem, *lcowbuilder.SandboxOptions, error) {
	return lcowbuilder.BuildSandboxConfig(ctx, "test-owner", bundle, shimOpts, spec)
}

// setupBootFiles creates a temporary directory containing the kernel and rootfs
// files that both document builders probe during boot configuration resolution.
func setupBootFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{
		vmutils.KernelFile,
		vmutils.UncompressedKernelFile,
		vmutils.InitrdFile,
		vmutils.VhdFile,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create boot file %s: %v", name, err)
		}
	}
	return dir
}

// jsonToString serializes v to indented JSON for test log output.
func jsonToString(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}
