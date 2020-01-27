package oci

import (
	"context"
	"testing"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func Test_ParseAnnotationsCPULimit(t *testing.T) {
	type tc struct {
		name string

		an, anv string

		slimit uint16

		def int32

		ev int32
	}
	cases := []tc{
		{
			name: "default_uvm",
			an:   annotationProcessorLimit,
			def:  50,
			ev:   50,
		},
		{
			name:   "spec_uvm_zero",
			an:     annotationProcessorLimit,
			slimit: 0,
			def:    50,
			ev:     50,
		},
		{
			name:   "spec_uvm_max",
			an:     annotationProcessorLimit,
			slimit: 10000,
			def:    50,
			ev:     50,
		},
		{
			name:   "spec_uvm_val",
			an:     annotationProcessorLimit,
			slimit: 60,
			def:    50,
			ev:     600, // slimit * 10 for single instance
		},
		{
			name: "annotation_uvm_zero",
			an:   annotationProcessorLimit,
			anv:  "0",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_uvm_max",
			an:   annotationProcessorLimit,
			anv:  "100000",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_uvm_val",
			an:   annotationProcessorLimit,
			anv:  "60",
			def:  50,
			ev:   60,
		},
		{
			name: "default_container",
			an:   AnnotationContainerProcessorLimit,
			def:  50,
			ev:   50,
		},
		{
			name:   "spec_container_zero",
			an:     AnnotationContainerProcessorLimit,
			slimit: 0,
			def:    50,
			ev:     50,
		},
		{
			name:   "spec_container_max",
			an:     AnnotationContainerProcessorLimit,
			slimit: 10000,
			def:    50,
			ev:     50,
		},
		{
			name:   "spec_container_val",
			an:     AnnotationContainerProcessorLimit,
			slimit: 60,
			def:    50,
			ev:     60,
		},
		{
			name: "annotation_container_zero",
			an:   AnnotationContainerProcessorLimit,
			anv:  "0",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_container_max",
			an:   AnnotationContainerProcessorLimit,
			anv:  "10000",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_container_val",
			an:   AnnotationContainerProcessorLimit,
			anv:  "60",
			def:  50,
			ev:   60,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			s := specs.Spec{}
			if c.anv != "" {
				s.Annotations = make(map[string]string)
				s.Annotations[c.an] = c.anv
			}
			if c.slimit != 0 {
				s.Windows = &specs.Windows{
					Resources: &specs.WindowsResources{
						CPU: &specs.WindowsCPUResources{
							Maximum: &c.slimit,
						},
					},
				}
			}
			actual := ParseAnnotationsCPULimit(ctx, &s, c.an, c.def)
			if actual != c.ev {
				t.Fatalf("expected value: %d, got: %d", c.ev, actual)
			}
		})
	}
}

func Test_ParseAnnotationsCPUWeight(t *testing.T) {
	type tc struct {
		name string

		an, anv string

		sweight uint16

		def int32

		ev int32
	}
	cases := []tc{
		{
			name: "default_uvm",
			an:   annotationProcessorWeight,
			def:  50,
			ev:   50,
		},
		{
			name:    "spec_uvm_zero",
			an:      annotationProcessorWeight,
			sweight: 0,
			def:     50,
			ev:      50,
		},
		{
			name:    "spec_uvm_default",
			an:      annotationProcessorWeight,
			sweight: 100,
			def:     50,
			ev:      50,
		},
		{
			name:    "spec_uvm_val",
			an:      annotationProcessorWeight,
			sweight: 60,
			def:     50,
			ev:      60,
		},
		{
			name: "annotation_uvm_zero",
			an:   annotationProcessorWeight,
			anv:  "0",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_uvm_default",
			an:   annotationProcessorWeight,
			anv:  "100",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_uvm_val",
			an:   annotationProcessorWeight,
			anv:  "60",
			def:  50,
			ev:   60,
		},
		{
			name: "default_container",
			an:   AnnotationContainerProcessorWeight,
			def:  50,
			ev:   50,
		},
		{
			name:    "spec_container_zero",
			an:      AnnotationContainerProcessorWeight,
			sweight: 0,
			def:     50,
			ev:      50,
		},
		{
			name:    "spec_container_default",
			an:      AnnotationContainerProcessorWeight,
			sweight: 100,
			def:     50,
			ev:      50,
		},
		{
			name:    "spec_container_val",
			an:      AnnotationContainerProcessorWeight,
			sweight: 60,
			def:     50,
			ev:      60,
		},
		{
			name: "annotation_container_zero",
			an:   AnnotationContainerProcessorWeight,
			anv:  "0",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_container_default",
			an:   AnnotationContainerProcessorWeight,
			anv:  "100",
			def:  50,
			ev:   50,
		},
		{
			name: "annotation_container_val",
			an:   AnnotationContainerProcessorWeight,
			anv:  "60",
			def:  50,
			ev:   60,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			s := specs.Spec{}
			if c.anv != "" {
				s.Annotations = make(map[string]string)
				s.Annotations[c.an] = c.anv
			}
			if c.sweight != 0 {
				s.Windows = &specs.Windows{
					Resources: &specs.WindowsResources{
						CPU: &specs.WindowsCPUResources{
							Shares: &c.sweight,
						},
					},
				}
			}
			actual := ParseAnnotationsCPUWeight(ctx, &s, c.an, c.def)
			if actual != c.ev {
				t.Fatalf("expected value: %d, got: %d", c.ev, actual)
			}
		})
	}
}

func Test_SpecUpdate_MemorySize_WithAnnotation_WithOpts(t *testing.T) {

	opts := &runhcsopts.Options{
		VmMemorySizeInMb: 3072,
	}
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Annotations: map[string]string{
			annotationMemorySizeInMB: "2048",
		},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[annotationMemorySizeInMB] != "2048" {
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

	if updatedSpec.Annotations[annotationMemorySizeInMB] != "3072" {
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
			annotationProcessorCount: "8",
		},
	}
	updatedSpec := UpdateSpecFromOptions(*s, opts)

	if updatedSpec.Annotations[annotationProcessorCount] != "8" {
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

	if updatedSpec.Annotations[annotationProcessorCount] != "4" {
		t.Fatal("should have updated annotation to default when annotation is not provided in the spec")
	}
}
