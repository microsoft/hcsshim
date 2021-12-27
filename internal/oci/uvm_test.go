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
		Annotations: map[string]string{},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("could not generate creation options from spec: %v", err)
	}

	lopts := (opts).(*uvm.OptionsLCOW)
	dopts := uvm.NewDefaultOptionsLCOW(t.Name(), "")

	// output handler equality is always false, so set to nil
	lopts.OutputHandler = nil
	dopts.OutputHandler = nil

	if !cmp.Equal(*lopts, *dopts) {
		t.Fatalf("should not have updated create options from default when no annotation are provided:\n%s", cmp.Diff(lopts, dopts))
	}
}

func Test_SpecToUVMCreateOptions_Default_WCOW(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
		Annotations: map[string]string{},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("could not generate creation options from spec: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	dopts := uvm.NewDefaultOptionsWCOW(t.Name(), "")

	if !cmp.Equal(*wopts, *dopts) {
		t.Fatalf("should not have updated create options from default when no annotation are provided:\n%s", cmp.Diff(wopts, dopts))
	}
}

func Test_SpecToUVMCreateOptions_Common_LCOW(t *testing.T) {
	cpugroupid := "1"
	lowmmiogap := 1024
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Annotations: map[string]string{
			annotations.ProcessorCount:             "8",
			annotations.CPUGroupID:                 cpugroupid,
			annotations.DisableWriteableFileShares: "true",
			annotations.MemoryLowMMIOGapInMB:       fmt.Sprint(lowmmiogap),
		},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("could not generate creation options from spec: %v", err)
	}

	lopts := (opts).(*uvm.OptionsLCOW)
	// todo: move these all into subtests with t.Run?
	if lopts.LowMMIOGapInMB != uint64(lowmmiogap) {
		t.Fatalf("should have updated creation options low MMIO Gap when annotation is provided: %v != %v", lopts.LowMMIOGapInMB, lowmmiogap)
	}
	if lopts.ProcessorCount != 8 {
		t.Fatalf("should have updated creation options processor count when annotation is provided: %v != 8", lopts.ProcessorCount)
	}
	if lopts.CPUGroupID != cpugroupid {
		t.Fatalf("should have updated creation options CPU group Id when annotation is provided: %v != %v", lopts.CPUGroupID, cpugroupid)
	}
	if !lopts.NoWritableFileShares {
		t.Fatal("should have disabled writable in shares creation when annotation is provided")
	}
}

func Test_SpecToUVMCreateOptions_Common_WCOW(t *testing.T) {
	cpugroupid := "1"
	lowmmiogap := 1024
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
		Annotations: map[string]string{
			annotations.ProcessorCount:             "8",
			annotations.CPUGroupID:                 cpugroupid,
			annotations.DisableWriteableFileShares: "true",
			annotations.MemoryLowMMIOGapInMB:       fmt.Sprint(lowmmiogap),
		},
	}

	opts, err := SpecToUVMCreateOpts(context.Background(), s, t.Name(), "")
	if err != nil {
		t.Fatalf("could not generate creation options from spec: %v", err)
	}

	wopts := (opts).(*uvm.OptionsWCOW)
	// todo: move these all into subtests with t.Run?
	if wopts.LowMMIOGapInMB != uint64(lowmmiogap) {
		t.Fatalf("should have updated creation options low MMIO Gap when annotation is provided: %v != %v", wopts.LowMMIOGapInMB, lowmmiogap)
	}
	if wopts.ProcessorCount != 8 {
		t.Fatalf("should have updated creation options processor count when annotation is provided: %v != 8", wopts.ProcessorCount)
	}
	if wopts.CPUGroupID != cpugroupid {
		t.Fatalf("should have updated creation options CPU group Id when annotation is provided: %v != %v", wopts.CPUGroupID, cpugroupid)
	}
	if !wopts.NoWritableFileShares {
		t.Fatal("should have disabled writable in shares creation when annotation is provided")
	}
}
