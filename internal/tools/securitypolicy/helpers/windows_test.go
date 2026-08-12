package helpers

import (
	"context"
	"strings"
	"testing"

	sp "github.com/Microsoft/hcsshim/pkg/securitypolicy"
)

func TestPolicyWindowsContainersFromConfigsRequiresImage(t *testing.T) {
	_, err := PolicyWindowsContainersFromConfigs(context.Background(), []sp.WindowsContainerConfig{{
		Command: []string{"cmd.exe"},
	}})
	if err == nil {
		t.Fatal("expected error when image_name is not set")
	}
	if !strings.Contains(err.Error(), "image_name") {
		t.Fatalf("unexpected error: %v", err)
	}
}
