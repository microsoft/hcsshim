package oci

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

func Test_ProccessAnnotations_Expansion(t *testing.T) {
	// suppress warnings raised by process annotation
	defer func(l logrus.Level) {
		logrus.SetLevel(l)
	}(logrus.GetLevel())
	logrus.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()

	tests := []struct {
		name string
		spec specs.Spec
	}{
		{
			name: "lcow",
			spec: specs.Spec{
				Linux: &specs.Linux{},
			},
		},
		{
			name: "wcow-hypervisor",
			spec: specs.Spec{
				Windows: &specs.Windows{
					HyperV: &specs.WindowsHyperV{},
				},
			},
		},
		{
			name: "wcow-process",
			spec: specs.Spec{
				Windows: &specs.Windows{},
			},
		},
	}

	for _, tt := range tests {
		// test correct expansion
		for _, v := range []string{"true", "false"} {
			t.Run(tt.name+"_disable_unsafe_"+v, func(subtest *testing.T) {
				tt.spec.Annotations = map[string]string{
					annotations.DisableUnsafeOperations: v,
				}

				err := ProcessAnnotations(ctx, &tt.spec)
				if err != nil {
					subtest.Fatalf("could not update spec from options: %v", err)
				}

				for _, k := range annotations.AnnotationExpansions[annotations.DisableUnsafeOperations] {
					if vv := tt.spec.Annotations[k]; vv != v {
						subtest.Fatalf("annotation %q was incorrectly expanded to %q, expected %q", k, vv, v)
					}
				}
			})
		}

		// test errors raised on conflict
		t.Run(tt.name+"_disable_unsafe_error", func(subtest *testing.T) {
			tt.spec.Annotations = map[string]string{
				annotations.DisableUnsafeOperations:   "true",
				annotations.DisableWritableFileShares: "false",
			}

			errExp := fmt.Sprintf("could not expand %q into %q",
				annotations.DisableUnsafeOperations,
				annotations.DisableWritableFileShares)

			err := ProcessAnnotations(ctx, &tt.spec)
			if !errors.Is(err, ErrAnnotationExpansionConflict) {
				t.Fatalf("UpdateSpecFromOptions should have failed with %q, actual was %v", errExp, err)
			}
		})
	}
}
