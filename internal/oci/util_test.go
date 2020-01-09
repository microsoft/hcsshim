package oci

import (
	"testing"

	runhcsopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func Test_IsLCOW_WCOW(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{},
	}
	if IsLCOW(s) {
		t.Fatal("should not have returned LCOW spec for WCOW config")
	}
}

func Test_IsLCOW_WCOW_Isolated(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if IsLCOW(s) {
		t.Fatal("should not have returned LCOW spec for WCOW isolated config")
	}
}

func Test_IsLCOW_Success(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsLCOW(s) {
		t.Fatal("should have returned LCOW spec")
	}
}

func Test_IsLCOW_NoWindows_Success(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
	}
	if !IsLCOW(s) {
		t.Fatal("should have returned LCOW spec")
	}
}

func Test_IsLCOW_Neither(t *testing.T) {
	s := &specs.Spec{}

	if IsLCOW(s) {
		t.Fatal("should not have returned LCOW spec for neither config")
	}
}

func Test_IsWCOW_Success(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{},
	}
	if !IsWCOW(s) {
		t.Fatal("should have returned WCOW spec for WCOW config")
	}
}

func Test_IsWCOW_Isolated_Success(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsWCOW(s) {
		t.Fatal("should have returned WCOW spec for WCOW isolated config")
	}
}

func Test_IsWCOW_LCOW(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if IsWCOW(s) {
		t.Fatal("should not have returned WCOW spec for LCOW config")
	}
}

func Test_IsWCOW_LCOW_NoWindows_Success(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
	}
	if IsWCOW(s) {
		t.Fatal("should not have returned WCOW spec for LCOW config")
	}
}

func Test_IsWCOW_Neither(t *testing.T) {
	s := &specs.Spec{}

	if IsWCOW(s) {
		t.Fatal("should not have returned WCOW spec for neither config")
	}
}

func Test_IsIsolated_WCOW(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{},
	}
	if IsIsolated(s) {
		t.Fatal("should not have returned isolated for WCOW config")
	}
}

func Test_IsIsolated_WCOW_Isolated(t *testing.T) {
	s := &specs.Spec{
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsIsolated(s) {
		t.Fatal("should have returned isolated for WCOW isolated config")
	}
}

func Test_IsIsolated_LCOW(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
		Windows: &specs.Windows{
			HyperV: &specs.WindowsHyperV{},
		},
	}
	if !IsIsolated(s) {
		t.Fatal("should have returned isolated for LCOW config")
	}
}

func Test_IsIsolated_LCOW_NoWindows(t *testing.T) {
	s := &specs.Spec{
		Linux: &specs.Linux{},
	}
	if !IsIsolated(s) {
		t.Fatal("should have returned isolated for LCOW config")
	}
}

func Test_IsIsolated_Neither(t *testing.T) {
	s := &specs.Spec{}

	if IsIsolated(s) {
		t.Fatal("should have not have returned isolated for neither config")
	}
}

func Test_ValidateSupportedMounts_NilOpts(t *testing.T) {
	s := specs.Spec{}
	err := ValidateSupportedMounts(s, nil)
	if err != nil {
		t.Fatal("should not have failed")
	}
}

func Test_ValidateSupportedMounts_DefaultOpts(t *testing.T) {
	s := specs.Spec{}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}
}

func Test_ValidateSupportedMounts_WCOW_Bind_File(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "",
				Source: "C:\\test.txt",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableBindMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_WCOW_Bind_Folder(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "",
				Source: "C:\\test",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableBindMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_LCOW_Bind_File(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "bind",
				Source: "/test.txt",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableBindMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_LCOW_Bind_Folder(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "bind",
				Source: "/test",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableBindMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_Physical_Disk(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "physical-disk",
				Source: "\\\\.\\PHYSICALDRIVE0",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisablePhysicalDiskMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_Virtual_Disk(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "virtual-disk",
				Source: "C:\\test.vhd",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableVirtualDiskMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_Automanage_Virtual_Disk(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "automanage-virtual-disk",
				Source: "C:\\test.vhd",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableVirtualDiskMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_WCOW_Pipe(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "",
				Source: "\\\\.\\pipe\\test",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisablePipeMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_WCOW_Sandbox(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "",
				Source: "sandbox://test",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableSandboxMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}

func Test_ValidateSupportedMounts_LCOW_Sandbox(t *testing.T) {
	s := specs.Spec{
		Mounts: []specs.Mount{
			{
				Type:   "bind",
				Source: "sandbox:///test",
			},
		},
	}
	o := &runhcsopts.Options{}
	err := ValidateSupportedMounts(s, o)
	if err != nil {
		t.Fatal("should not have failed")
	}

	o.DisableSandboxMounts = true
	err = ValidateSupportedMounts(s, o)
	if err == nil {
		t.Fatal("should have failed")
	}
}
