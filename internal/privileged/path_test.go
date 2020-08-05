package privileged

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchPath(t *testing.T) {
	if _, err := searchPath("C:\\windows\\system32", "ping"); err != nil {
		t.Fatalf("failed to find executable in path: %s", err)
	}
}

func TestFindExecutable(t *testing.T) {
	if _, err := findExecutable("ping", ""); err != nil {
		t.Fatalf("failed to find executable: %s", err)
	}

	if _, err := findExecutable("ping.exe", ""); err != nil {
		t.Fatalf("failed to find executable: %s", err)
	}

	if _, err := findExecutable("C:\\windows\\system32\\ping", ""); err != nil {
		t.Fatalf("failed to find executable: %s", err)
	}

	if _, err := findExecutable("C:\\windows\\system32\\ping.exe", ""); err != nil {
		t.Fatalf("failed to find executable: %s", err)
	}

	// Create nested directory structure with blank test executables.
	path, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to make temporary directory: %s", err)
	}
	defer os.RemoveAll(path)

	_, err = os.Create(filepath.Join(path, "test.exe"))
	if err != nil {
		t.Fatalf("failed to create test executable: %s", err)
	}

	nestedPath := filepath.Join(path, "\\path\\to\\binary")
	if err := os.MkdirAll(nestedPath, 0700); err != nil {
		t.Fatalf("failed to create nested directory structure: %s", err)
	}

	_, err = os.Create(filepath.Join(nestedPath, "test.exe"))
	if err != nil {
		t.Fatalf("failed to create test executable: %s", err)
	}

	if _, err := findExecutable("test.exe", path); err != nil {
		t.Fatalf("failed to find executable: %s", err)
	}

	if _, err := findExecutable("path\\to\\binary\\test.exe", path); err != nil {
		t.Fatalf("failed to find executable: %s", err)
	}
}
