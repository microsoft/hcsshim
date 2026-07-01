//go:build windows && lcow

package vm

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/Microsoft/hcsshim/internal/builder/vm/lcow"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/vm/vmmanager"

	runhcsoptions "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	vmsandbox "github.com/Microsoft/hcsshim/sandbox-spec/vm/v2"

	"github.com/Microsoft/go-winio"
	"go.uber.org/mock/gomock"
)

// ─── CreateVM Error Paths (LCOW-only) ──────────────────────────────────────────

func TestCreateVM(t *testing.T) {
	ctx := context.Background()
	opts := &CreateOptions{ID: "test-vm", Owner: "test-owner"}

	t.Run("buildHCSConfig_fails/state_stays_not_created", func(t *testing.T) {
		c := New()
		orig := buildSandboxConfig
		t.Cleanup(func() { buildSandboxConfig = orig })
		buildSandboxConfig = func(_ context.Context, _ string, _ string, _ *runhcsoptions.Options, _ *vmsandbox.Spec) (*hcsschema.ComputeSystem, *lcow.SandboxOptions, error) {
			return nil, nil, errors.New("bad sandbox spec")
		}

		err := c.CreateVM(ctx, opts)
		if err == nil {
			t.Fatal("expected error when buildHCSConfig fails")
		}
		if c.State() != StateNotCreated {
			t.Errorf("expected StateNotCreated, got %s", c.State())
		}
	})

	t.Run("createVM_fails/state_stays_not_created", func(t *testing.T) {
		c := New()
		orig := buildSandboxConfig
		t.Cleanup(func() { buildSandboxConfig = orig })
		buildSandboxConfig = func(_ context.Context, _ string, _ string, _ *runhcsoptions.Options, _ *vmsandbox.Spec) (*hcsschema.ComputeSystem, *lcow.SandboxOptions, error) {
			return &hcsschema.ComputeSystem{}, &lcow.SandboxOptions{}, nil
		}

		origCreate := createVM
		t.Cleanup(func() { createVM = origCreate })
		createVM = func(_ context.Context, _ string, _ *hcsschema.ComputeSystem) (*vmmanager.UtilityVM, error) {
			return nil, errors.New("HCS create failed")
		}

		err := c.CreateVM(ctx, opts)
		if err == nil {
			t.Fatal("expected error when createVM fails")
		}
		if c.State() != StateNotCreated {
			t.Errorf("expected StateNotCreated, got %s", c.State())
		}
	})
}

// ─── StartVM: AddSecurityPolicy Error Path (LCOW-only) ────────────────────────

func TestStartVM_AddSecurityPolicyFails(t *testing.T) {
	ctx := context.Background()

	c, uvm, guest := newControllerWithState(t, StateCreated)
	uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()
	uvm.EXPECT().ID().Return("test-vm-id").AnyTimes()

	swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
		return &fakeListener{}, nil
	})
	uvm.EXPECT().AcceptConnection(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&fakeConn{}, nil).AnyTimes()

	guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
	uvm.EXPECT().Start(gomock.Any()).Return(nil)
	expectVMRunsUntilCleanup(t, uvm, guest)
	guest.EXPECT().CreateConnection(gomock.Any(), gomock.Any()).Return(nil)

	// Set confidential config so buildConfidentialOptions returns non-nil.
	c.sandboxOptions = &lcow.SandboxOptions{
		ConfidentialConfig: &lcow.ConfidentialConfig{
			SecurityPolicy:         "test-policy",
			SecurityPolicyEnforcer: "test-enforcer",
			UvmReferenceInfoFile:   "test-ref",
		},
	}

	// Inject parseUVMReferenceInfo to succeed.
	origParse := parseUVMReferenceInfo
	t.Cleanup(func() { parseUVMReferenceInfo = origParse })
	parseUVMReferenceInfo = func(_ context.Context, _, _ string) (string, error) {
		return "encoded-ref-info", nil
	}

	// AddSecurityPolicy returns an error.
	guest.EXPECT().AddSecurityPolicy(gomock.Any(), gomock.Any()).Return(errors.New("security policy failed"))

	err := c.StartVM(ctx, &StartOptions{})
	if err == nil {
		t.Fatal("expected error when AddSecurityPolicy fails")
	}
	if c.State() != StateInvalid {
		t.Errorf("expected StateInvalid, got %s", c.State())
	}
}

// ─── StartVM: buildConfidentialOptions Error Path (LCOW-only) ──────────────────

func TestStartVM_BuildConfidentialOptionsFails(t *testing.T) {
	ctx := context.Background()

	c, uvm, guest := newControllerWithState(t, StateCreated)
	uvm.EXPECT().RuntimeID().Return(testGUID).AnyTimes()
	uvm.EXPECT().ID().Return("test-vm-id").AnyTimes()

	swapListenHVSock(t, func(_ *winio.HvsockAddr) (net.Listener, error) {
		return &fakeListener{}, nil
	})
	uvm.EXPECT().AcceptConnection(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&fakeConn{}, nil).AnyTimes()

	guest.EXPECT().PrepareConnection(gomock.Any()).Return(nil)
	uvm.EXPECT().Start(gomock.Any()).Return(nil)
	expectVMRunsUntilCleanup(t, uvm, guest)
	guest.EXPECT().CreateConnection(gomock.Any(), gomock.Any()).Return(nil)

	// Set confidential config so buildConfidentialOptions is called.
	c.sandboxOptions = &lcow.SandboxOptions{
		ConfidentialConfig: &lcow.ConfidentialConfig{
			UvmReferenceInfoFile: "test-ref",
		},
	}

	// Inject parseUVMReferenceInfo to fail.
	origParse := parseUVMReferenceInfo
	t.Cleanup(func() { parseUVMReferenceInfo = origParse })
	parseUVMReferenceInfo = func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("parse reference info failed")
	}

	err := c.StartVM(ctx, &StartOptions{})
	if err == nil {
		t.Fatal("expected error when buildConfidentialOptions fails")
	}
	if c.State() != StateInvalid {
		t.Errorf("expected StateInvalid, got %s", c.State())
	}
}
