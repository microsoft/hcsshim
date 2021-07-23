package jobcontainers

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func assertStr(t *testing.T, a string, b string) {
	if !strings.EqualFold(a, b) {
		t.Fatalf("expected %s, got %s", a, b)
	}
}

func TestSearchPath(t *testing.T) {
	// Testing that relative paths work.
	_, err := searchPathForExe("windows\\system32\\ping", "C:\\")
	if err != nil {
		t.Fatal(err)
	}

	_, err = searchPathForExe("system32\\ping", "C:\\windows")
	if err != nil {
		t.Fatal(err)
	}

	_, err = searchPathForExe("ping", "C:\\windows\\system32")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetApplicationName(t *testing.T) {
	expected := "C:\\WINDOWS\\system32\\ping.exe"

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	path, _, err := getApplicationName("ping", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	path, _, err = getApplicationName("./ping", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	path, _, err = getApplicationName(".\\ping", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	// Test that we only find the first element of the commandline if the binary exists.
	path, _, err = getApplicationName("ping test", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	// Test quoted application name with an argument afterwards.
	path, cmdLine, err := getApplicationName("\"ping\" 127.0.0.1", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	args := splitArgs(cmdLine)
	cmd := &exec.Cmd{
		Path: path,
		Args: args,
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
