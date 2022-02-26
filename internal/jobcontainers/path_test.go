package jobcontainers

import (
	"os"
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

type config struct {
	name                    string
	commandLine             string
	workDir                 string
	pathEnv                 string
	expectedApplicationName string
	expectedCmdline         string
}

func runGetApplicationNameTests(t *testing.T, tests []*config) {
	for _, cfg := range tests {
		t.Run(cfg.name, func(t *testing.T) {
			path, cmdLine, err := getApplicationName(cfg.commandLine, cfg.workDir, cfg.pathEnv)
			if err != nil {
				t.Fatal(err)
			}
			assertStr(t, cfg.expectedCmdline, cmdLine)
			assertStr(t, cfg.expectedApplicationName, path)
		})
	}
}

func TestGetApplicationNamePing(t *testing.T) {
	expected := "C:\\WINDOWS\\system32\\ping.exe"

	tests := []*config{
		{
			name:                    "Ping",
			commandLine:             "ping",
			workDir:                 "C:\\",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "ping",
			expectedApplicationName: expected,
		},
		{
			name:                    "Ping_Relative_Forward_Slash",
			commandLine:             "system32/ping",
			workDir:                 "C:\\windows\\",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "system32/ping",
			expectedApplicationName: expected,
		},
		{
			name:                    "Ping_Relative_Back_Slash",
			commandLine:             "system32\\ping",
			workDir:                 "C:\\windows",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "system32\\ping",
			expectedApplicationName: expected,
		},
		{
			name:                    "Ping_Cwd_Windows_Directory",
			commandLine:             "system32\\ping",
			workDir:                 "C:\\Windows",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "system32\\ping",
			expectedApplicationName: expected,
		},
		{
			name:                    "Ping_With_Cwd",
			commandLine:             "cmd /c ping 127.0.0.1",
			workDir:                 "C:\\",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "cmd /c ping 127.0.0.1",
			expectedApplicationName: "C:\\windows\\system32\\cmd.exe",
		},
		{
			name:                    "Ping_With_Cwd_Relative_Path",
			commandLine:             "system32\\cmd /c ping 127.0.0.1",
			workDir:                 "C:\\windows\\",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "system32\\cmd /c ping 127.0.0.1",
			expectedApplicationName: "C:\\windows\\system32\\cmd.exe",
		},
		{
			name:                    "Ping_With_Space",
			commandLine:             "ping test",
			workDir:                 "C:\\",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "ping test",
			expectedApplicationName: expected,
		},
		{
			name:                    "Ping_With_Quote",
			commandLine:             "\"ping\" 127.0.0.1",
			workDir:                 "C:\\",
			pathEnv:                 "C:\\windows\\system32",
			expectedCmdline:         "\"ping\" 127.0.0.1",
			expectedApplicationName: expected,
		},
	}

	runGetApplicationNameTests(t, tests)
}

func TestGetApplicationNameRandomBinary(t *testing.T) {
	tempDir := t.TempDir()

	// Create fake executables in a temporary directory to use for the below tests.
	testExe := filepath.Join(tempDir, "test.exe")
	f1, err := os.Create(testExe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f1.Close() })

	test2Exe := filepath.Join(tempDir, "test 2.exe")
	f2, err := os.Create(test2Exe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f2.Close() })

	exeWithSpace := filepath.Join(tempDir, "exe with space.exe")
	f3, err := os.Create(exeWithSpace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f3.Close() })

	tests := []*config{
		// See if we can successfully find "exe with space.exe" with no quoting, it should first try "exe.exe", then "exe with.exe" and then finally
		// "exe with space.exe"
		{
			name:                    "Spaces_With_No_Quoting",
			commandLine:             "exe with space.exe",
			workDir:                 filepath.Dir(testExe),
			pathEnv:                 "",
			expectedCmdline:         "\"exe with space.exe\"",
			expectedApplicationName: exeWithSpace,
		},
		// See if we can successfully find "exe with space.exe" with quoting, it should try "exe with space.exe" only.
		{
			name:                    "Spaces_With_Quoting",
			commandLine:             "\"exe with space.exe\"",
			workDir:                 filepath.Dir(testExe),
			pathEnv:                 "",
			expectedCmdline:         "\"exe with space.exe\"",
			expectedApplicationName: exeWithSpace,
		},
		// Try a quoted commandline, so that we find the actual "C:\rest\of\the\path\test 2.exe" binary
		{
			name:                    "Test2_Binary_With_Quotes",
			commandLine:             "\"test 2.exe\"",
			workDir:                 filepath.Dir(test2Exe),
			pathEnv:                 "",
			expectedCmdline:         "\"test 2.exe\"",
			expectedApplicationName: test2Exe,
		},
		// We should find the test.exe binary, and the 2 will be treated as an argument in this case
		{
			name:                    "Test2_Binary_No_Quotes",
			commandLine:             "test 2",
			workDir:                 filepath.Dir(test2Exe),
			pathEnv:                 "",
			expectedCmdline:         "test 2",
			expectedApplicationName: testExe,
		},
		// Test finding test binary with no file extension
		{
			name:                    "Test_Binary_No_File_Extension",
			commandLine:             testExe[0 : len(testExe)-4],
			workDir:                 filepath.Dir(testExe),
			pathEnv:                 "",
			expectedCmdline:         testExe[0 : len(testExe)-4],
			expectedApplicationName: testExe,
		},
		// Test finding the test binary with the PATH containing the directory it lives in.
		{
			name:                    "Test_Binary_With_Path_Containing_Location",
			commandLine:             "test",
			workDir:                 "C:\\",
			pathEnv:                 filepath.Dir(testExe),
			expectedCmdline:         "test",
			expectedApplicationName: testExe,
		},
	}

	runGetApplicationNameTests(t, tests)
}
