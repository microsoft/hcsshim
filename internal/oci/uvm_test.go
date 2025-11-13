//go:build windows

package oci

import (
	"context"
	"fmt"
	"testing"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/Microsoft/hcsshim/internal/uvm"
	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func Test_SpecUpdate_MemorySize_WithAnnotation_WithOpts(t *testing.T) {
	opts := &runhcsopts.Options{
		VmMemorySizeInMb: 3072,
	}
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Annotations: map[string]string{
			annotations.MemorySizeInMB: "2048",
		},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[annotations.MemorySizeInMB] != "2048" {
		t.Fatal("should not have updated annotation to default when annotation is provided in the spec")
	}
}

func Test_SpecUpdate_MemorySize_NoAnnotation_WithOpts(t *testing.T) {
	opts := &runhcsopts.Options{
		VmMemorySizeInMb: 3072,
	}
	s := &specs.Spec{
		Linux:       &specs.Linux{},
		Annotations: map[string]string{},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[annotations.MemorySizeInMB] != "3072" {
		t.Fatal("should have updated annotation to default when annotation is not provided in the spec")
	}
}

func Test_SpecUpdate_ProcessorCount_WithAnnotation_WithOpts(t *testing.T) {
	opts := &runhcsopts.Options{
		VmProcessorCount: 4,
	}
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Annotations: map[string]string{
			annotations.ProcessorCount: "8",
		},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[annotations.ProcessorCount] != "8" {
		t.Fatal("should not have updated annotation to default when annotation is provided in the spec")
	}
}

func Test_SpecUpdate_ProcessorCount_NoAnnotation_WithOpts(t *testing.T) {
	opts := &runhcsopts.Options{
		VmProcessorCount: 4,
	}
	s := &specs.Spec{
		Linux:       &specs.Linux{},
		Annotations: map[string]string{},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[annotations.ProcessorCount] != "4" {
		t.Fatal("should have updated annotation to default when annotation is not provided in the spec")
	}
}

func Test_SpecToUVMCreateOptions_Default_LCOW(t *testing.T) {
	s := &specs.Spec{
		Linux:       &specs.Linux{},
		Annotations: make(map[string]string),
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("could not generate creation options from spec: %v", err)
	}

	lopts := (opts).(*uvm.OptionsLCOW)
	dopts := uvm.NewDefaultOptionsLCOW(t.Name(), "")

	// output handler equality is always false, so set to nil
	lopts.OutputHandlerCreator = nil
	dopts.OutputHandlerCreator = nil

	if !cmp.Equal(*lopts, *dopts) {
		t.Fatalf("should not have updated create options from default when no annotation are provided:\n%s", cmp.Diff(lopts, dopts))
	}
}

func Test_SpecToUVMCreateOptions_Default_WCOW(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
		Annotations: make(map[string]string),
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("could not generate creation options from spec: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	dopts := uvm.NewDefaultOptionsWCOW(t.Name(), "")

	// output handler equality is always false, so set to nil
	wopts.OutputHandlerCreator = nil
	dopts.OutputHandlerCreator = nil

	if !cmp.Equal(*wopts, *dopts) {
		t.Fatalf("should not have updated create options from default when no annotation are provided:\n%s", cmp.Diff(wopts, dopts))
	}
}

func Test_SpecToUVMCreateOptions_WCOW_Confidential_Defaults(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{HyperV: &specs.WindowsHyperV{}},
		Annotations: map[string]string{
			annotations.WCOWSecurityPolicy: "test-policy",
		},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("unexpected error generating UVM opts: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	if !wopts.SecurityPolicyEnabled {
		t.Fatal("SecurityPolicyEnabled should be true when WCOWSecurityPolicy is set")
	}
	// Writable EFI should default to false unless explicitly enabled
	if wopts.WritableEFI {
		t.Fatal("WritableEFI should default to false when not specified")
	}
	if wopts.MemorySizeInMB != 2048 {
		t.Fatalf("expected MemorySizeInMB to default to 2048, got %d", wopts.MemorySizeInMB)
	}
	if wopts.AllowOvercommit {
		t.Fatal("AllowOvercommit must be false for confidential pods by default")
	}
	if wopts.DisableSecureBoot {
		t.Fatal("DisableSecureBoot should default to false when not specified")
	}
	if wopts.SecurityPolicyEnforcer != "" {
		t.Fatalf("expected empty SecurityPolicyEnforcer by default, got %q", wopts.SecurityPolicyEnforcer)
	}
	if wopts.GuestStateFilePath != uvm.GetDefaultConfidentialVMGSPath() {
		t.Fatalf("expected GuestStateFilePath to default to %q, got %q", uvm.GetDefaultConfidentialVMGSPath(), wopts.GuestStateFilePath)
	}
	if wopts.IsolationType != "SecureNestedPaging" {
		t.Fatalf("expected SecureNestedPaging IsolationType by default, got %q", wopts.IsolationType)
	}
}

func Test_SpecToUVMCreateOptions_WCOW_Confidential_WritableEFI_Enabled(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{HyperV: &specs.WindowsHyperV{}},
		Annotations: map[string]string{
			annotations.WCOWSecurityPolicy: "test-policy",
			annotations.WCOWWritableEFI:    "true",
		},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("unexpected error generating UVM opts: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	if !wopts.WritableEFI {
		t.Fatal("WritableEFI should be true when WCOWWritableEFI annotation is set to true")
	}
}

func Test_SpecToUVMCreateOptions_WCOW_NonConfidential_WritableEFI_Enabled(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{HyperV: &specs.WindowsHyperV{}},
		Annotations: map[string]string{
			// No WCOWSecurityPolicy means non-confidential path
			annotations.WCOWWritableEFI: "true",
		},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("unexpected error generating UVM opts: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	// WritableEFI should be respected for non-confidential as well
	if !wopts.WritableEFI {
		t.Fatal("WritableEFI should be true for non-confidential when WCOWWritableEFI is set")
	}
}

func Test_SpecToUVMCreateOptions_WCOW_Confidential_ErrorOnLowMemory(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{HyperV: &specs.WindowsHyperV{}},
		Annotations: map[string]string{
			annotations.WCOWSecurityPolicy: "test-policy",
			annotations.MemorySizeInMB:     "1024",
		},
	}

	if _, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), ""); err == nil {
		t.Fatal("expected error for confidential pods with MemorySizeInMB < 2048, got nil")
	}
}

func Test_SpecToUVMCreateOptions_WCOW_Confidential_Overrides(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{HyperV: &specs.WindowsHyperV{}},
		Annotations: map[string]string{
			annotations.WCOWSecurityPolicy:         "test-policy",
			annotations.MemorySizeInMB:             "4096",
			annotations.AllowOvercommit:            "false",
			annotations.WCOWSecurityPolicyEnforcer: "rego",
			annotations.WCOWDisableSecureBoot:      "true",
			annotations.WCOWGuestStateFile:         "C:\\custom\\cwcow.vmgs",
			annotations.WCOWIsolationType:          "VirtualizationBasedSecurity",
		},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("unexpected error generating UVM opts: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	if wopts.MemorySizeInMB != 4096 {
		t.Fatalf("expected MemorySizeInMB=4096, got %d", wopts.MemorySizeInMB)
	}
	if wopts.AllowOvercommit {
		t.Fatal("AllowOvercommit should be false when explicitly set to false")
	}
	if wopts.SecurityPolicyEnforcer != "rego" {
		t.Fatalf("expected SecurityPolicyEnforcer=rego, got %q", wopts.SecurityPolicyEnforcer)
	}
	if !wopts.DisableSecureBoot {
		t.Fatal("DisableSecureBoot should be true when specified")
	}
	if wopts.GuestStateFilePath != "C:\\custom\\cwcow.vmgs" {
		t.Fatalf("expected GuestStateFilePath to match override, got %q", wopts.GuestStateFilePath)
	}
	if wopts.IsolationType != "VirtualizationBasedSecurity" {
		t.Fatalf("expected IsolationType=VirtualizationBasedSecurity, got %q", wopts.IsolationType)
	}
}

func Test_SpecToUVMCreateOptions_Common(t *testing.T) {
	cpugroupid := "1"
	lowmmiogap := 1024
	as := map[string]string{
		annotations.ProcessorCount:            "8",
		annotations.CPUGroupID:                cpugroupid,
		annotations.DisableWritableFileShares: "true",
		annotations.MemoryLowMMIOGapInMB:      fmt.Sprint(lowmmiogap),
	}

	tests := []struct {
		name    string
		spec    specs.Spec
		extract func(interface{}) *uvm.Options
	}{
		{
			name: "lcow",
			spec: specs.Spec{
				Linux: &specs.Linux{},
			},
			// generics would be nice ...
			extract: func(i interface{}) *uvm.Options {
				o := (i).(*uvm.OptionsLCOW)
				return o.Options
			},
		},
		{
			name: "wcow-hypervisor",
			spec: specs.Spec{
				Windows: &specs.Windows{
					HyperV: &specs.WindowsHyperV{},
				},
			},
			extract: func(i interface{}) *uvm.Options {
				o := (i).(*uvm.OptionsWCOW)
				return o.Options
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.spec.Annotations = as
			opts, err := SpecToUVMCreateOpts(context.Background(), &tt.spec, t.Name(), "")
			if err != nil {
				t.Fatalf("could not generate creation options from spec: %v", err)
			}

			// get the underlying uvm.Options from uvm.Options[LW]COW
			copts := tt.extract(opts)
			if copts.LowMMIOGapInMB != uint64(lowmmiogap) {
				t.Fatalf("should have updated creation options low MMIO Gap when annotation is provided: %v != %v", copts.LowMMIOGapInMB, lowmmiogap)
			}
			if copts.ProcessorCount != 8 {
				t.Fatalf("should have updated creation options processor count when annotation is provided: %v != 8", copts.ProcessorCount)
			}
			if copts.CPUGroupID != cpugroupid {
				t.Fatalf("should have updated creation options CPU group Id when annotation is provided: %v != %v", copts.CPUGroupID, cpugroupid)
			}
			if !copts.NoWritableFileShares {
				t.Fatal("should have disabled writable in shares creation when annotation is provided")
			}
		})
	}
}
