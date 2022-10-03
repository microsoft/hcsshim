//go:build windows

package internal

import (
	"encoding/json"
	"os"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func GetDefaultWindowsSpec(t *testing.T) *specs.Spec {
	t.Helper()
	content, err := os.ReadFile(`assets\defaultwindowsspec.json`)
	if err != nil {
		t.Fatalf("failed to read defaultwindowsspec.json: %s", err.Error())
	}
	spec := specs.Spec{}
	if err := json.Unmarshal(content, &spec); err != nil {
		t.Fatalf("failed to unmarshal contents of defaultwindowsspec.json: %s", err.Error())
	}
	return &spec
}
