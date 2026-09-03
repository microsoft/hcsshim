//go:build linux
// +build linux

package hcsv2

import (
	"reflect"
	"strings"
	"testing"

	oci "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func countLdconfig(args []string) int {
	n := 0
	for _, a := range args {
		if strings.HasPrefix(a, "--ldconfig=") {
			n++
		}
	}
	return n
}

// TestNvidiaConfigureArgs_LdconfigInjection is a regression test for argument
// injection via the untrusted GPU capabilities annotation. Setting it to
// "utility,compute,ldconfig=@<payload>" used to append the value verbatim after
// the fixed --ldconfig=@/sbin/ldconfig, producing a second --ldconfig that won
// under nvidia-container-cli's last-flag-wins parsing.
func TestNvidiaConfigureArgs_LdconfigInjection(t *testing.T) {
	const payload = "utility,compute,ldconfig=@/attacker/controlled/payload"

	// Demonstrate the previous behavior: the naive comma-split the hook used to
	// perform yields a second, attacker-controlled --ldconfig after the default.
	legacy := []string{"--ldconfig=@/sbin/ldconfig"}
	for _, c := range strings.Split(payload, ",") {
		legacy = append(legacy, "--"+c)
	}
	if got := countLdconfig(legacy); got != 2 {
		t.Fatalf("precondition: legacy construction should inject a second --ldconfig, got %d", got)
	}
	if legacy[len(legacy)-1] != "--ldconfig=@/attacker/controlled/payload" {
		t.Fatalf("precondition: legacy construction should leave the injected --ldconfig last, got %q", legacy[len(legacy)-1])
	}

	// New behavior: the same payload is rejected before any argv is produced.
	spec := &oci.Spec{Annotations: map[string]string{
		annotations.ContainerGPUCapabilities: payload,
	}}
	if _, err := nvidiaConfigureArgs("/path/generichook", "--debug=/tmp/log", spec); err == nil {
		t.Fatal("nvidiaConfigureArgs() accepted ldconfig injection payload, want error")
	}
}

// TestNvidiaConfigureArgs_Valid confirms a legitimate capability set keeps
// exactly the single fixed --ldconfig and appends the expected flags in order.
func TestNvidiaConfigureArgs_Valid(t *testing.T) {
	spec := &oci.Spec{Annotations: map[string]string{
		annotations.ContainerGPUCapabilities: "compute,utility",
	}}
	args, err := nvidiaConfigureArgs("/path/generichook", "--debug=/tmp/log", spec)
	if err != nil {
		t.Fatalf("nvidiaConfigureArgs() error = %v", err)
	}
	if got := countLdconfig(args); got != 1 {
		t.Fatalf("expected exactly one --ldconfig, got %d in %v", got, args)
	}
	want := []string{
		"/path/generichook",
		"nvidia-container-cli",
		"--debug=/tmp/log",
		"--no-pivot",
		"configure",
		"--ldconfig=@/sbin/ldconfig",
		"--compute",
		"--utility",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("nvidiaConfigureArgs() = %v, want %v", args, want)
	}
}

func TestNvidiaCapabilityArgs(t *testing.T) {
	tests := []struct {
		name         string
		capabilities string
		want         []string
		wantErr      string
	}{
		{
			name:         "valid capabilities",
			capabilities: "all,compat32,compute,display,graphics,ngx,utility,video",
			want:         []string{"--all", "--compat32", "--compute", "--display", "--graphics", "--ngx", "--utility", "--video"},
		},
		{
			name:         "valued option rejected",
			capabilities: "compute,no-cgroups",
			wantErr:      `unsupported NVIDIA GPU capability "no-cgroups"`,
		},
		{
			name:         "argument injection",
			capabilities: "utility,compute,ldconfig=@/attacker/controlled/payload",
			wantErr:      `unsupported NVIDIA GPU capability "ldconfig=@/attacker/controlled/payload"`,
		},
		{
			name:         "unknown capability",
			capabilities: "network",
			wantErr:      `unsupported NVIDIA GPU capability "network"`,
		},
		{
			name:         "empty capability",
			capabilities: "",
			wantErr:      `unsupported NVIDIA GPU capability ""`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nvidiaCapabilityArgs(test.capabilities)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("nvidiaCapabilityArgs() error = %v, want error containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("nvidiaCapabilityArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("nvidiaCapabilityArgs() = %v, want %v", got, test.want)
			}
		})
	}
}
