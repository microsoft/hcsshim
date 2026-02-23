package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	iannotations "github.com/Microsoft/hcsshim/internal/annotations"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func TestProccessAnnotations_HostProcessContainer(t *testing.T) {
	// suppress warnings raised by process annotation
	defer func(l logrus.Level) {
		logrus.SetLevel(l)
	}(logrus.GetLevel())
	logrus.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()

	testAnnotations := []struct {
		name string
		an   map[string]string
		errs []error
	}{
		//
		// valid cases
		//

		{
			name: "DisableUnsafeOperations-DisableHostProcessContainer",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "true",
				annotations.DisableHostProcessContainer: "true",
				annotations.HostProcessContainer:        "false",
			},
		},
		{
			name: "HostProcessContainer",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "false",
				annotations.DisableHostProcessContainer: "false",
				annotations.HostProcessContainer:        "true",
			},
		},
		{
			name: "All false",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "false",
				annotations.DisableHostProcessContainer: "false",
				annotations.HostProcessContainer:        "false",
			},
		},

		//
		// invalid
		//

		{
			name: "DisableUnsafeOperations-DisableHostProcessContainer-HostProcessContainer",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "true",
				annotations.DisableHostProcessContainer: "true",
				annotations.HostProcessContainer:        "true",
			},
			errs: []error{ErrGenericAnnotationConflict},
		},
		{
			name: "DisableUnsafeOperations",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "true",
				annotations.DisableHostProcessContainer: "false",
				annotations.HostProcessContainer:        "false",
			},
			errs: []error{ErrAnnotationExpansionConflict},
		},
		{
			name: "DisableHostProcessContainer",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "false",
				annotations.DisableHostProcessContainer: "true",
				annotations.HostProcessContainer:        "false",
			},
			errs: []error{ErrAnnotationExpansionConflict},
		},

		// expansion both conflicts and causes conflict
		{
			name: "DisableUnsafeOperations-HostProcessContainer",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "true",
				annotations.DisableHostProcessContainer: "false",
				annotations.HostProcessContainer:        "true",
			},
			errs: []error{ErrAnnotationExpansionConflict},
		},
		{
			name: "DisableHostProcessContainer-HostProcessContainer",
			an: map[string]string{
				annotations.DisableUnsafeOperations:     "false",
				annotations.DisableHostProcessContainer: "true",
				annotations.HostProcessContainer:        "true",
			},
			errs: []error{ErrAnnotationExpansionConflict, ErrGenericAnnotationConflict},
		},
	}

	for _, tt := range testAnnotations {
		t.Run(tt.name, func(t *testing.T) {
			spec := specs.Spec{
				Annotations: tt.an,
			}

			err := ProcessAnnotations(ctx, spec.Annotations)
			if err != nil && len(tt.errs) == 0 {
				t.Fatalf("ProcessAnnotations should have succeeded, instead got %v", err)
			}
			if len(tt.errs) > 0 {
				if err == nil {
					t.Fatalf("ProcessAnnotations succeeded; should have failed with %v", tt.errs)
				}
				for _, e := range tt.errs {
					if !errors.Is(err, e) {
						t.Fatalf("ProcessAnnotations should failed with %v", e)
					}
				}
			}
		})
	}
}

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

				err := ProcessAnnotations(ctx, tt.spec.Annotations)
				if err != nil {
					subtest.Fatalf("could not update spec from options: %v", err)
				}

				ae := annotations.AnnotationExpansionMap()
				for _, k := range ae[annotations.DisableUnsafeOperations] {
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

			err := ProcessAnnotations(ctx, tt.spec.Annotations)
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
				"some-random-annotation":                                "random",
				"not-microsoft.virtualmachine.wcow.additional-reg-keys": "this is fake",
				iannotations.AdditionalRegistryValues:                   v,
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

func TestParseHVSocketServiceTable(t *testing.T) {
	ctx := context.Background()

	toString := func(t *testing.T, v hcsschema.HvSocketServiceConfig) string {
		t.Helper()

		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "")

		if err := enc.Encode(v); err != nil {
			t.Fatalf("encode %v to JSON: %v", v, err)
		}

		return strings.TrimSpace(buf.String())
	}

	g1 := "0b52781f-b24d-5685-ddf6-69830ed40ec3"
	g2 := "00000000-0000-0000-0000-000000000000"

	defaultConfig := hcsschema.HvSocketServiceConfig{
		AllowWildcardBinds:     true,
		BindSecurityDescriptor: "D:P(A;;FA;;;WD)",
	}
	defaultConfigStr := toString(t, defaultConfig)

	disabledConfig := hcsschema.HvSocketServiceConfig{
		Disabled: true,
	}
	disabledConfigStr := toString(t, disabledConfig)

	for _, tt := range []struct {
		name string
		give map[string]string
		want map[string]hcsschema.HvSocketServiceConfig
	}{
		{
			name: "empty",
		},
		{
			name: "single",
			give: map[string]string{
				iannotations.UVMHyperVSocketConfigPrefix + g1: defaultConfigStr,
			},
			want: map[string]hcsschema.HvSocketServiceConfig{
				g1: defaultConfig,
			},
		},
		{
			name: "invalid guid",
			give: map[string]string{
				iannotations.UVMHyperVSocketConfigPrefix + "not-a-guid": defaultConfigStr,
			},
		},
		{
			name: "invalid config",
			give: map[string]string{
				iannotations.UVMHyperVSocketConfigPrefix + g1: `["not", "a", "valid", "config"]`,
			},
		},
		{
			name: "override",
			give: map[string]string{
				iannotations.UVMHyperVSocketConfigPrefix + g1:                  defaultConfigStr,
				iannotations.UVMHyperVSocketConfigPrefix + strings.ToUpper(g1): defaultConfigStr,
			},
			want: map[string]hcsschema.HvSocketServiceConfig{
				g1: defaultConfig,
			},
		},
		{
			name: "multiple",
			give: map[string]string{
				iannotations.UVMHyperVSocketConfigPrefix + strings.ToUpper(g1): defaultConfigStr,
				iannotations.UVMHyperVSocketConfigPrefix + g2:                  disabledConfigStr,

				iannotations.UVMHyperVSocketConfigPrefix + g1:           `["not", "a", "valid", "config"]`,
				iannotations.UVMHyperVSocketConfigPrefix + "not-a-guid": defaultConfigStr,
				"also.not-a-guid": disabledConfigStr,
			},
			want: map[string]hcsschema.HvSocketServiceConfig{
				g1: defaultConfig,
				g2: disabledConfig,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			annots := map[string]string{
				"some-random-annotation":                               "random",
				"io.microsoft.virtualmachine.hv-socket.service-table":  "should be ignored",
				"not-microsoft.virtualmachine.hv-socket.service-table": "this is fake",
			}
			maps.Copy(annots, tt.give)
			t.Logf("annotations:\n%v", annots)

			rvs := ParseHVSocketServiceTable(ctx, annots)
			t.Logf("got %v", rvs)
			want := tt.want
			if want == nil {
				want = map[string]hcsschema.HvSocketServiceConfig{}
			}
			if diff := cmp.Diff(want, rvs); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
