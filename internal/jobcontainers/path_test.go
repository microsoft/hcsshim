package jobcontainers

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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

func TestGetApplicationNamePing(t *testing.T) {
	expected := "C:\\WINDOWS\\system32\\ping.exe"

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	pathEnv := os.Getenv("PATH")

	path, _, err := getApplicationName("ping", cwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	path, _, err = getApplicationName("./ping", cwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	path, _, err = getApplicationName(".\\ping", cwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	// Test relative path with different cwd
	newCwd := `C:\Windows\`
	_, _, err = getApplicationName("./system32/ping", newCwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	pingWithCmd := "cmd /c ping 127.0.0.1"
	path, cmdLine, err := getApplicationName("cmd /c ping 127.0.0.1", cwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, cmdLine, pingWithCmd)
	assertStr(t, "C:\\windows\\system32\\cmd.exe", path)

	// Test that we only find the first element of the commandline if the binary exists.
	path, _, err = getApplicationName("ping test", cwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	// Test quoted application name with an argument afterwards.
	path, cmdLine, err = getApplicationName("\"ping\" 127.0.0.1", cwd, pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, expected, path)

	cmd := &exec.Cmd{
		Path: path,
		Args: splitArgs(cmdLine),
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestGetApplicationNameRandomBinary(t *testing.T) {
	pathEnv := os.Getenv("PATH")

	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create fake executables in a temporary directory to use for the below tests.
	testExe := filepath.Join(tempDir, "test.exe")
	_, err = os.Create(testExe)
	if err != nil {
		t.Fatal(err)
	}

	test2Exe := filepath.Join(tempDir, "test 2.exe")
	_, err = os.Create(test2Exe)
	if err != nil {
		t.Fatal(err)
	}

	exeWithSpace := filepath.Join(tempDir, "exe with space.exe")
	_, err = os.Create(exeWithSpace)
	if err != nil {
		t.Fatal(err)
	}

	// See if we can successfully find "exe with space.exe" with no quoting, it should first try "exe.exe", then "exe with.exe" and then finally
	// "exe with space.exe"
	path, _, err := getApplicationName("exe with space.exe", filepath.Dir(testExe), os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, exeWithSpace, path)

	// See if we can successfully find "exe with space.exe" with quoting, it should try "exe with space.exe" only.
	path, _, err = getApplicationName("\"exe with space.exe\"", filepath.Dir(testExe), os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, exeWithSpace, path)

	// Try a quoted commandline, so that we find the actual "C:\rest\of\the\path\test 2.exe" binary
	path, _, err = getApplicationName("\"test 2.exe\"", filepath.Dir(test2Exe), os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, test2Exe, path)

	// We should find the test.exe binary, and the 2 will be treated as an argument in this case
	path, _, err = getApplicationName("test 2", filepath.Dir(test2Exe), os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, testExe, path)

	// Test relative path with the current working directory set to the directory that contains the binary.
	path, _, err = getApplicationName("./test.exe", filepath.Dir(testExe), pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, testExe, path)

	// Test relative path with backslashes with the current working directory set to the directory that contains the binary.
	path, _, err = getApplicationName(".\\test.exe", filepath.Dir(testExe), pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, testExe, path)

	// Test no file extension
	path, _, err = getApplicationName(testExe[0:len(testExe)-4], filepath.Dir(testExe), pathEnv)
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, testExe, path)

	// Add test binary path to PATH and try to find it by just 'test.exe'
	if err := os.Setenv("PATH", os.Getenv("PATH")+filepath.Dir(testExe)); err != nil {
		t.Fatal(err)
	}
	path, _, err = getApplicationName("test.exe", filepath.Dir(testExe), os.Getenv("PATH"))
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, testExe, path)

}
