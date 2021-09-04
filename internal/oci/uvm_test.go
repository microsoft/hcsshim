package oci

import (
	"testing"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func Test_SpecUpdate_MemorySize_WithAnnotation_WithOpts(t *testing.T) {

	opts := &runhcsopts.Options{
		VmMemorySizeInMb: 3072,
	}
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Annotations: map[string]string{
			AnnotationMemorySizeInMB: "2048",
		},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[AnnotationMemorySizeInMB] != "2048" {
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

	if updatedSpec.Annotations[AnnotationMemorySizeInMB] != "3072" {
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
			AnnotationProcessorCount: "8",
		},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[AnnotationProcessorCount] != "8" {
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

	if updatedSpec.Annotations[AnnotationProcessorCount] != "4" {
		t.Fatal("should have updated annotation to default when annotation is not provided in the spec")
	}
}
