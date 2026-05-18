//go:build windows && lcow

package linuxcontainer

import (
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/pkg/annotations"
)

// TestSanitizeSpec_CPUDefaults verifies that explicitly zeroed CPU period and
// quota are replaced with safe defaults, while non-zero values pass through.
func TestSanitizeSpec_CPUDefaults(t *testing.T) {
	t.Parallel()

	period := func(v uint64) *uint64 { return &v }
	quota := func(v int64) *int64 { return &v }

	tests := []struct {
		name       string
		period     *uint64
		quota      *int64
		wantPeriod uint64
		wantQuota  int64
	}{
		{
			name:       "zero-period-and-quota",
			period:     period(0),
			quota:      quota(0),
			wantPeriod: 100000,
			wantQuota:  -1,
		},
		{
			name:       "non-zero-values-unchanged",
			period:     period(50000),
			quota:      quota(25000),
			wantPeriod: 50000,
			wantQuota:  25000,
		},
		{
			name:       "nil-period-and-quota",
			period:     nil,
			quota:      nil,
			wantPeriod: 0, // unused; checked via nil
			wantQuota:  0, // unused; checked via nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &specs.Spec{Linux: &specs.Linux{}, Windows: &specs.Windows{}}
			spec.Linux.Resources = &specs.LinuxResources{
				CPU: &specs.LinuxCPU{Period: tt.period, Quota: tt.quota},
			}

			got, err := sanitizeSpec(t.Context(), spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			cpu := got.Linux.Resources.CPU
			if tt.period == nil {
				if cpu.Period != nil {
					t.Errorf("Period = %v, want nil", *cpu.Period)
				}
			} else if *cpu.Period != tt.wantPeriod {
				t.Errorf("Period = %d, want %d", *cpu.Period, tt.wantPeriod)
			}

			if tt.quota == nil {
				if cpu.Quota != nil {
					t.Errorf("Quota = %v, want nil", *cpu.Quota)
				}
			} else if *cpu.Quota != tt.wantQuota {
				t.Errorf("Quota = %d, want %d", *cpu.Quota, tt.wantQuota)
			}
		})
	}
}

// TestSanitizeSpec_ClearsGCSManagedResources verifies that cgroups path and
// resource types managed by the GCS are removed from the sanitized spec.
func TestSanitizeSpec_ClearsGCSManagedResources(t *testing.T) {
	t.Parallel()
	classID := uint32(1)
	cpuShares := uint64(512)
	pidsLimit := int64(100)
	spec := &specs.Spec{Linux: &specs.Linux{}, Windows: &specs.Windows{}}
	spec.Linux.CgroupsPath = "/sys/fs/cgroup/test"
	spec.Linux.Resources = &specs.LinuxResources{
		Devices:        []specs.LinuxDeviceCgroup{{Allow: true}},
		Pids:           &specs.LinuxPids{Limit: &pidsLimit},
		BlockIO:        &specs.LinuxBlockIO{},
		HugepageLimits: []specs.LinuxHugepageLimit{{Pagesize: "2MB", Limit: 1024}},
		Network:        &specs.LinuxNetwork{ClassID: &classID},
		// CPU should be preserved.
		CPU: &specs.LinuxCPU{Shares: &cpuShares},
	}

	got, err := sanitizeSpec(t.Context(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Linux.CgroupsPath != "" {
		t.Errorf("CgroupsPath = %q, want empty", got.Linux.CgroupsPath)
	}
	res := got.Linux.Resources
	if res.Devices != nil {
		t.Error("Devices should be nil")
	}
	if res.Pids != nil {
		t.Error("Pids should be nil")
	}
	if res.BlockIO != nil {
		t.Error("BlockIO should be nil")
	}
	if res.HugepageLimits != nil {
		t.Error("HugepageLimits should be nil")
	}
	if res.Network != nil {
		t.Error("Network should be nil")
	}
	// CPU must survive the clear.
	if res.CPU == nil || res.CPU.Shares == nil || *res.CPU.Shares != 512 {
		t.Error("CPU.Shares should be preserved as 512")
	}
}

// TestSanitizeSpec_NilsHooksAndSeccomp verifies that hooks are always removed
// and seccomp is removed only for privileged containers.
func TestSanitizeSpec_NilsHooksAndSeccomp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		privileged  bool
		wantSeccomp bool
	}{
		{name: "non-privileged", privileged: false, wantSeccomp: true},
		{name: "privileged", privileged: true, wantSeccomp: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &specs.Spec{Linux: &specs.Linux{}, Windows: &specs.Windows{}}
			spec.Hooks = &specs.Hooks{
				CreateRuntime: []specs.Hook{{Path: "/bin/hook"}},
			}
			spec.Linux.Seccomp = &specs.LinuxSeccomp{
				DefaultAction: specs.ActErrno,
			}
			if tt.privileged {
				spec.Annotations = map[string]string{
					annotations.LCOWPrivileged: "true",
				}
			}

			got, err := sanitizeSpec(t.Context(), spec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Hooks != nil {
				t.Error("Hooks should be nil")
			}
			hasSeccomp := got.Linux.Seccomp != nil
			if hasSeccomp != tt.wantSeccomp {
				t.Errorf("Seccomp present = %v, want %v", hasSeccomp, tt.wantSeccomp)
			}
		})
	}
}

// TestSanitizeSpec_DeepCopy confirms mutations to the sanitized spec do not
// affect the original.
func TestSanitizeSpec_DeepCopy(t *testing.T) {
	t.Parallel()
	orig := &specs.Spec{Linux: &specs.Linux{}, Windows: &specs.Windows{}}
	orig.Hostname = "original"

	got, err := sanitizeSpec(t.Context(), orig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got.Hostname = "mutated"
	if orig.Hostname != "original" {
		t.Error("mutation of sanitized spec leaked to the original")
	}
}

// TestSanitizeSpec_NilResources verifies sanitizeSpec succeeds when
// Linux.Resources is nil (no CPU/resource rewriting needed).
func TestSanitizeSpec_NilResources(t *testing.T) {
	t.Parallel()
	spec := &specs.Spec{Linux: &specs.Linux{}, Windows: &specs.Windows{}}
	spec.Linux.Resources = nil

	got, err := sanitizeSpec(t.Context(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Linux.Resources != nil {
		t.Error("Resources should remain nil")
	}
}

// TestExtractWindowsFields verifies that only the network namespace and assigned
// devices are preserved from the Windows section.
func TestExtractWindowsFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		windows     *specs.Windows
		wantNil     bool
		wantNetwork bool
		wantDevices int
	}{
		{
			name:    "empty-windows",
			windows: &specs.Windows{},
			wantNil: true,
		},
		{
			name: "network-only",
			windows: &specs.Windows{
				Network: &specs.WindowsNetwork{NetworkNamespace: "ns1"},
			},
			wantNetwork: true,
		},
		{
			name: "devices-only",
			windows: &specs.Windows{
				Devices: []specs.WindowsDevice{{ID: "dev1"}},
			},
			wantDevices: 1,
		},
		{
			name: "network-and-devices",
			windows: &specs.Windows{
				Network: &specs.WindowsNetwork{NetworkNamespace: "ns2"},
				Devices: []specs.WindowsDevice{{ID: "d1"}, {ID: "d2"}},
			},
			wantNetwork: true,
			wantDevices: 2,
		},
		{
			name: "empty-network-namespace-ignored",
			windows: &specs.Windows{
				Network: &specs.WindowsNetwork{NetworkNamespace: ""},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec := &specs.Spec{Windows: tt.windows}
			got := extractWindowsFields(spec)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil Windows")
			}

			if tt.wantNetwork {
				if got.Network == nil || got.Network.NetworkNamespace == "" {
					t.Error("expected network namespace to be preserved")
				}
			} else if got.Network != nil {
				t.Error("expected no network")
			}

			if len(got.Devices) != tt.wantDevices {
				t.Errorf("Devices count = %d, want %d", len(got.Devices), tt.wantDevices)
			}
		})
	}
}

// TestGenerateContainerDocument_NilLinux verifies that generateContainerDocument
// returns an error when the Linux section is absent.
func TestGenerateContainerDocument_NilLinux(t *testing.T) {
	t.Parallel()
	c := &Controller{gcsPodID: "pod", gcsContainerID: "ctr"}
	spec := &specs.Spec{} // no Linux section

	_, err := c.generateContainerDocument(t.Context(), spec, nil, false)
	if err == nil {
		t.Fatal("expected error for nil Linux section")
	}
}
