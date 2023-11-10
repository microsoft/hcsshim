package oci

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func TestProccessAnnotations_Expansion(t *testing.T) {
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

func TestParseAdditionalRegistryValues(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name string
		give string
		want []hcsschema.RegistryValue
	}{
		{
			name: "empty",
		},
		{
			name: "nil",
			give: "null",
		},
		{
			name: "empty list",
			give: "[]",
		},
		{
			name: "invalid",
			give: "invalid",
		},
		{
			name: "nil key",
			give: `[
{"Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value" }
]`,
		},
		{
			name: "invalid hive",
			give: `[
{"Key": {"Hive": "Invalid", "Name": "software\\a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value" }
]`,
		},
		{
			name: "empty key name",
			give: `[
{"Key": {"Hive": "System"}, "Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value" }
]`,
		},
		{
			name: "empty name",
			give: `[
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"}, "Type": "String", "StringValue": "registry key value value" }
]`,
		},
		{
			name: "invalid type",
			give: `[
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "Invalid", "StringValue": "registry key value value" }
]`,
		},
		{
			name: "multiple types",
			give: `[
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value", "QWordValue": 1 }
]`,
		},
		{
			name: "denied",
			give: `[
{"Key": {"Hive": "System", "Name": "a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value"}
]`,
		},
		{
			name: "valid",
			give: `[
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value" },
{"Key": {"Hive": "System", "Name": "software\\another\\registry\\key"},
	"Name": "dwordRegistryKeyName", "Type": "DWord", "DWordValue": 1 }
]`,
			want: []hcsschema.RegistryValue{
				{
					Key: &hcsschema.RegistryKey{
						Hive: hcsschema.RegistryHive_SYSTEM,
						Name: "software\\a\\registry\\key",
					},
					Name:        "stringRegistryKeyName",
					Type_:       hcsschema.RegistryValueType_STRING,
					StringValue: "registry key value value",
				},
				{
					Key: &hcsschema.RegistryKey{
						Hive: hcsschema.RegistryHive_SYSTEM,
						Name: "software\\another\\registry\\key",
					},
					Name:       "dwordRegistryKeyName",
					Type_:      hcsschema.RegistryValueType_D_WORD,
					DWordValue: 1,
				},
			},
		},
		{
			name: "multiple",
			give: `[
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value" },
{"Key": {"Hive": "System"}, "Name": "stringRegistryKeyName", "Type": "String", "StringValue": "registry key value value" },
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"}, "Type": "String", "StringValue": "registry key value value" },
{"Key": {"Hive": "System", "Name": "software\\another\\registry\\key"}, "Name": "dwordRegistryKeyName", "Type": "DWord", "DWordValue": 1 },
{"Key": {"Hive": "System", "Name": "denied\\registry\\key"}, "Name": "dwordRegistryKeyName", "Type": "DWord", "DWordValue": 1 },
{"Key": {"Hive": "System", "Name": "software\\a\\registry\\key"},
	"Name": "stringRegistryKeyName", "Type": "Invalid", "StringValue": "registry key value value" }
]`,
			want: []hcsschema.RegistryValue{
				{
					Key: &hcsschema.RegistryKey{
						Hive: hcsschema.RegistryHive_SYSTEM,
						Name: "software\\a\\registry\\key",
					},
					Name:        "stringRegistryKeyName",
					Type_:       hcsschema.RegistryValueType_STRING,
					StringValue: "registry key value value",
				},
				{
					Key: &hcsschema.RegistryKey{
						Hive: hcsschema.RegistryHive_SYSTEM,
						Name: "software\\another\\registry\\key",
					},
					Name:       "dwordRegistryKeyName",
					Type_:      hcsschema.RegistryValueType_D_WORD,
					DWordValue: 1,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("registry values:\n%s", tt.give)
			v := strings.ReplaceAll(tt.give, "\n", "")
			rvs := parseAdditionalRegistryValues(ctx, map[string]string{
				iannotations.AdditionalRegistryValues: v,
			})
			want := tt.want
			if want == nil {
				want = []hcsschema.RegistryValue{}
			}
			if diff := cmp.Diff(want, rvs); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
