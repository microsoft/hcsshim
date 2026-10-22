package securitypolicy

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestConfigureRuntimeLogging(t *testing.T) {
	original := logrus.StandardLogger().Out
	t.Cleanup(func() { logrus.SetOutput(original) })
	for _, tc := range []struct {
		name     string
		enforcer SecurityPolicyEnforcer
		allowed  bool
	}{
		{"allow", &OpenDoorSecurityPolicyEnforcer{}, true},
		{"deny", &ClosedDoorSecurityPolicyEnforcer{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			called := false
			s := NewSecurityOptions(tc.enforcer, true, "", "", &output,
				WithRuntimeLoggingChanged(func(allowed bool) {
					called = true
					if allowed != tc.allowed {
						t.Fatalf("allowed = %v, want %v", allowed, tc.allowed)
					}
					want := io.Writer(io.Discard)
					if allowed {
						want = &output
					}
					if logrus.StandardLogger().Out != want {
						t.Fatal("callback and output writer decisions differ")
					}
				}))
			s.configureRuntimeLogging(context.Background())
			if !called {
				t.Fatal("callback not invoked")
			}
		})
	}
}

func TestRuntimeLoggingCallbackNotCalledOnRejectedPolicy(t *testing.T) {
	for _, alreadySet := range []bool{false, true} {
		s := NewSecurityOptions(&ClosedDoorSecurityPolicyEnforcer{}, alreadySet, "", "", io.Discard,
			WithRuntimeLoggingChanged(func(bool) {
				t.Error("rejected policy changed logging authorization")
			}))
		if err := s.SetConfidentialOptions(context.Background(), "", "invalid-base64!", "", ""); err == nil {
			t.Fatal("expected invalid or duplicate policy rejection")
		}
	}
}
