//go:build windows

package internal

import (
	"encoding/json"
	"os"
	"testing"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func GetDefaultLinuxSpec(t *testing.T) *specs.Spec {
	t.Helper()
	content, err := os.ReadFile(`assets\defaultlinuxspec.json`)
	if err != nil {
		t.Fatalf("failed to read defaultlinuxspec.json: %s", err.Error())
	}
	spec := specs.Spec{}
	if err := json.Unmarshal(content, &spec); err != nil {
		t.Fatalf("failed to unmarshal contents of defaultlinuxspec.json: %s", err.Error())
	}
	return &spec
}
