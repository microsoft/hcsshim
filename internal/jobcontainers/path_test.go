package jobcontainers

import (
	"os"
	"os/exec"
	"testing"
)

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
}

func TestGetApplicationName(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = getApplicationName("ping", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}

	// Test that we only find the first element of the commandline if the binary exists.
	_, _, err = getApplicationName("ping test", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}

	// Test quoted application name with an argument afterwards.
	path, cmdLine, err := getApplicationName("\"ping\" 127.0.0.1", cwd, os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}

	args := splitArgs(cmdLine)
	cmd := &exec.Cmd{
		Path: path,
		Args: args,
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
