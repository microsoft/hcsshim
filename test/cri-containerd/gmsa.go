//go:build functional
// +build functional

package cri_containerd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Generates a gmsa credential spec and places it at 'path'. Dir has to exist, it will not be created by
// New-CredentialSpec.
func generateCredSpec(path string) error {
	output, err := exec.Command(
		"powershell",
		"New-CredentialSpec",
		"-AccountName",
		gmsaAccount,
		"-Path",
		path,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create new credential spec (output: %s): %v", string(output), err)
	}
	return nil
}

// Tries to generate a cred spec to use for gmsa test cases. Returns the cred
// spec and an error if any.
func gmsaSetup(t *testing.T) string {
	csDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp directory: %s", err)
	}
	defer os.RemoveAll(csDir)
	csPath := filepath.Join(csDir, "credspec.json")
	if err := generateCredSpec(csPath); err != nil {
		t.Fatal(err)
	}
	credSpec, err := ioutil.ReadFile(csPath)
	if err != nil {
		t.Fatalf("failed to read credential spec: %s", err)
	}
	return string(credSpec)
}
