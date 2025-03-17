//go:build windows

package uvm

import (
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func Test_ParseNamedPipe(t *testing.T) {
	type testConfig struct {
		uvm             *UtilityVM
		mount           specs.Mount
		parsedNamedPipe NamedPipe
		expected        bool
		name            string
	}

	for _, tc := range []testConfig{
		// not a named pipe
		{
			mount: specs.Mount{
				Source:      `sandbox://path\to\sandboxMount`,
				Destination: `C:\path\in\container`,
			},
			expected: false,
			name:     "SandboxMount",
		},
		// named pipe for process isolated
		{
			mount: specs.Mount{
				Source:      `\\.\pipe\hostPipe`,
				Destination: `\\.\pipe\containerPipe`,
			},
			parsedNamedPipe: NamedPipe{
				HostPath:      `\\.\pipe\hostPipe`,
				ContainerPath: `containerPipe`,
			},
			expected: true,
			name:     "NamedPipeForArgon",
		},
		// named pipe over VSMB
		{
			uvm: &UtilityVM{
				operatingSystem: "windows",
			},
			mount: specs.Mount{
				Source:      `\\.\pipe\hostPipe`,
				Destination: `\\.\pipe\containerPipe`,
			},
			parsedNamedPipe: NamedPipe{
				HostPath:      vsmbSharePrefix + `IPC$\hostPipe`,
				ContainerPath: `containerPipe`,
			},
			expected: true,
			name:     "NamedPipeForXenon",
		},
		// UVM mount, not a named pipe
		{
			uvm: &UtilityVM{},
			mount: specs.Mount{
				Source:      `uvm://C:\path\in\uvm`,
				Destination: `C:\path\in\container`,
			},
			expected: false,
			name:     "UVMDirectoryMount",
		},
		// UVM named pipe
		{
			uvm: &UtilityVM{
				id:              "pod-id@vm",
				operatingSystem: "windows",
			},
			mount: specs.Mount{
				Source:      `uvm://\\.\pipe\uvmPipe`,
				Destination: `\\.\pipe\containerPipe`,
			},
			parsedNamedPipe: NamedPipe{
				HostPath:      `\\.\pipe\pod-id\uvmPipe`,
				ContainerPath: `containerPipe`,
				UVMPipe:       true,
			},
			expected: true,
			name:     "UVMNamedPipeMountWindows",
		},
		{
			uvm: &UtilityVM{
				operatingSystem: "linux",
			},
			mount: specs.Mount{
				Source:      `uvm://\\.\pipe\containerPipe`,
				Destination: `\\.\pipe\containerPipe`,
			},
			expected: false,
			name:     "UVMNamedPipeMountLinux",
		},
	} {
		t.Run(t.Name()+"_"+tc.name, func(t *testing.T) {
			np, ok := ParseNamedPipe(tc.uvm, tc.mount)
			if ok != tc.expected {
				t.Log(np)
				t.Fatalf("ParseNamedPipe failed: expected %v, got %v", tc.expected, ok)
			}
			if np != tc.parsedNamedPipe {
				t.Fatalf("ParseNamedPipe failed: expected %v, got %v", tc.parsedNamedPipe, np)
			}
		})
	}
}
