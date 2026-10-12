package securitypolicy

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Microsoft/hcsshim/pkg/annotations"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func TestWriteSecurityContextDirAMDCertificate(t *testing.T) {
	const lcowCert = "bGNvdy1jZXJ0"
	const wcowCert = "d2Nvdy1jZXJ0"
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		wantCert    string
	}{
		{
			name:        "LCOW",
			annotations: map[string]string{annotations.LCOWHostAMDCertificate: lcowCert},
			wantCert:    lcowCert,
		},
		{
			name:        "deprecated generic alias",
			annotations: map[string]string{annotations.HostAMDCertificate: lcowCert}, //nolint:staticcheck // SA1019: intentionally test compatibility with the deprecated alias.
			wantCert:    lcowCert,
		},
		{
			name:        "WCOW fallback",
			annotations: map[string]string{annotations.WCOWHostAMDCertificate: wcowCert},
			wantCert:    wcowCert,
		},
		{
			name: "LCOW takes precedence",
			annotations: map[string]string{
				annotations.LCOWHostAMDCertificate: lcowCert,
				annotations.WCOWHostAMDCertificate: wcowCert,
			},
			wantCert: lcowCert,
		},
		{
			name: "empty LCOW falls back to WCOW",
			annotations: map[string]string{
				annotations.LCOWHostAMDCertificate: "",
				annotations.WCOWHostAMDCertificate: wcowCert,
			},
			wantCert: wcowCert,
		},
		{name: "nil annotations"},
		{name: "empty annotations", annotations: map[string]string{}},
		{
			name: "empty certificates",
			annotations: map[string]string{
				annotations.LCOWHostAMDCertificate: "",
				annotations.WCOWHostAMDCertificate: "",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, withOtherContext := range []bool{false, true} {
				name := "certificate only"
				if withOtherContext {
					name = "with other security context"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					spec := &specs.Spec{
						Root:        &specs.Root{Path: root},
						Process:     &specs.Process{Env: []string{"EXISTING=value"}},
						Annotations: tc.annotations,
					}
					enforcer := &OpenDoorSecurityPolicyEnforcer{}
					options := &SecurityOptions{PolicyEnforcer: enforcer}
					wantFiles := map[string]string{}
					if tc.wantCert != "" {
						wantFiles[HostAMDCertFilename] = tc.wantCert
					}
					if withOtherContext {
						enforcer.encodedSecurityPolicy = "cG9saWN5"
						options.UvmReferenceInfo = "cmVmZXJlbmNl"
						options.UvmHashEnvelopeReferenceInfo = "aGFzaC1lbnZlbG9wZQ=="
						options.tcbReferenceInfo = []byte("tcb")
						options.platformReferenceInfo = []byte("platform")
						wantFiles[PolicyFilename] = enforcer.encodedSecurityPolicy
						wantFiles[ReferenceInfoFilename] = options.UvmReferenceInfo
						wantFiles[HashEnvelopeReferenceInfoFilename] = options.UvmHashEnvelopeReferenceInfo
						wantFiles[TCBReferenceInfoFilename] = base64.StdEncoding.EncodeToString(options.tcbReferenceInfo)
						wantFiles[PlatformReferenceInfoFilename] = base64.StdEncoding.EncodeToString(options.platformReferenceInfo)
					}

					dir, err := options.WriteSecurityContextDir(spec)
					if err != nil {
						t.Fatal(err)
					}
					wantEnv := []string{"EXISTING=value"}
					if len(wantFiles) == 0 {
						if dir != "" {
							t.Fatalf("unexpected security context directory %q", dir)
						}
						entries, err := os.ReadDir(root)
						if err != nil {
							t.Fatal(err)
						}
						if len(entries) != 0 {
							t.Fatalf("unexpected rootfs entries: %v", entries)
						}
					} else {
						if dir == "" || filepath.Dir(dir) != root {
							t.Fatalf("expected security context directory under %q, got %q", root, dir)
						}
						entries, err := os.ReadDir(dir)
						if err != nil {
							t.Fatal(err)
						}
						if len(entries) != len(wantFiles) {
							t.Fatalf("expected %d context files, got %d", len(wantFiles), len(entries))
						}
						for filename, want := range wantFiles {
							got, err := os.ReadFile(filepath.Join(dir, filename))
							if err != nil {
								t.Fatal(err)
							}
							if string(got) != want {
								t.Errorf("%s: got %q, want %q", filename, got, want)
							}
						}
						wantEnv = append(wantEnv, "UVM_SECURITY_CONTEXT_DIR=/"+filepath.Base(dir))
					}
					if !reflect.DeepEqual(spec.Process.Env, wantEnv) {
						t.Errorf("environment: got %v, want %v", spec.Process.Env, wantEnv)
					}
				})
			}
		})
	}
}
