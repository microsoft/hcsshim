package securitypolicy

import (
	"context"
	"io"
	"testing"
)

// TestLockDown_SwapsEnforcerToClosedDoor verifies that LockDown replaces
// the active enforcer with a ClosedDoorSecurityPolicyEnforcer.
func TestLockDown_SwapsEnforcerToClosedDoor(t *testing.T) {
	opts := NewSecurityOptions(&OpenDoorSecurityPolicyEnforcer{}, true, "", io.Discard)

	if _, ok := opts.PolicyEnforcer.(*OpenDoorSecurityPolicyEnforcer); !ok {
		t.Fatalf("setup: expected initial enforcer to be OpenDoor, got %T", opts.PolicyEnforcer)
	}

	opts.LockDown(context.Background())

	if _, ok := opts.PolicyEnforcer.(*ClosedDoorSecurityPolicyEnforcer); !ok {
		t.Errorf("after LockDown: expected ClosedDoor enforcer, got %T", opts.PolicyEnforcer)
	}
}

// TestLockDown_Idempotent verifies that LockDown is a no-op once already locked.
// The same enforcer instance is preserved on the second call.
func TestLockDown_Idempotent(t *testing.T) {
	opts := NewSecurityOptions(&OpenDoorSecurityPolicyEnforcer{}, true, "", io.Discard)
	ctx := context.Background()

	opts.LockDown(ctx)
	firstLocked := opts.PolicyEnforcer

	opts.LockDown(ctx)
	secondLocked := opts.PolicyEnforcer

	if firstLocked != secondLocked {
		t.Errorf("LockDown should be idempotent: enforcer pointer changed between calls (%p -> %p)", firstLocked, secondLocked)
	}
}

// TestLockDown_SetsPolicyEnforcerSet verifies that LockDown marks the policy
// as set, so that a subsequent SetConfidentialOptions call cannot replace
// the closed-door enforcer with a permissive one.
//
// SetConfidentialOptions short-circuits with an error when PolicyEnforcerSet
// is already true; this is the mechanism that makes LockDown sticky against
// future policy-install attempts regardless of call order.
func TestLockDown_SetsPolicyEnforcerSet(t *testing.T) {
	// Construct with PolicyEnforcerSet=false to simulate LockDown being
	// called before the initial policy was loaded.
	opts := NewSecurityOptions(&OpenDoorSecurityPolicyEnforcer{}, false, "", io.Discard)

	if opts.PolicyEnforcerSet {
		t.Fatal("setup: expected PolicyEnforcerSet=false before LockDown")
	}

	opts.LockDown(context.Background())

	if !opts.PolicyEnforcerSet {
		t.Error("expected PolicyEnforcerSet=true after LockDown so subsequent SetConfidentialOptions refuses to install a policy")
	}
}

// TestLockDown_StickyAgainstSetConfidentialOptions verifies the end-to-end
// stickiness property: after LockDown, SetConfidentialOptions refuses to
// install a new (potentially permissive) policy and the enforcer remains
// closed-door.
func TestLockDown_StickyAgainstSetConfidentialOptions(t *testing.T) {
	opts := NewSecurityOptions(&OpenDoorSecurityPolicyEnforcer{}, false, "", io.Discard)
	ctx := context.Background()

	opts.LockDown(ctx)

	// Try to install a fresh policy after lockdown. The actual policy
	// content does not matter — SetConfidentialOptions should refuse on
	// the PolicyEnforcerSet check before touching anything else.
	err := opts.SetConfidentialOptions(ctx, "", "", "")
	if err == nil {
		t.Fatal("expected SetConfidentialOptions to refuse after LockDown")
	}

	if _, ok := opts.PolicyEnforcer.(*ClosedDoorSecurityPolicyEnforcer); !ok {
		t.Errorf("after LockDown + SetConfidentialOptions: enforcer was replaced; got %T, want ClosedDoor", opts.PolicyEnforcer)
	}
}

// TestLockDown_StickyFromBootClosedDoor covers the boot-time case the
// sidecar actually hits: the enforcer is already a ClosedDoor instance (the
// PSP fail-close default in cmd/gcs-sidecar/main.go) and PolicyEnforcerSet
// is still false because no user policy has been loaded yet. LockDown must
// still flip PolicyEnforcerSet so that a follow-up SetConfidentialOptions
// is refused, otherwise a permissive policy could replace the closed door.
func TestLockDown_StickyFromBootClosedDoor(t *testing.T) {
	opts := NewSecurityOptions(&ClosedDoorSecurityPolicyEnforcer{}, false, "", io.Discard)
	ctx := context.Background()

	opts.LockDown(ctx)

	if !opts.PolicyEnforcerSet {
		t.Fatal("expected PolicyEnforcerSet=true after LockDown on a boot-time ClosedDoor enforcer; otherwise SetConfidentialOptions would still accept a fresh policy")
	}

	err := opts.SetConfidentialOptions(ctx, "", "", "")
	if err == nil {
		t.Fatal("expected SetConfidentialOptions to refuse after LockDown on a boot-time ClosedDoor enforcer")
	}

	if _, ok := opts.PolicyEnforcer.(*ClosedDoorSecurityPolicyEnforcer); !ok {
		t.Errorf("after LockDown + SetConfidentialOptions: enforcer was replaced; got %T, want ClosedDoor", opts.PolicyEnforcer)
	}
}
