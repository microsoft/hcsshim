package exec

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/hcsshim/internal/conpty"
	"github.com/Microsoft/hcsshim/internal/jobobject"
)

func TestExec(t *testing.T) {
	// Exec a simple process and wait for exit.
	e, err := New(
		`C:\Windows\System32\ping.exe`,
		"ping 127.0.0.1",
		WithEnv(os.Environ()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	err = e.Wait()
	if err != nil {
		t.Fatalf("error waiting for process: %v", err)
	}
	t.Logf("exit code was: %d", e.ExitCode())
}

func TestExecWithDir(t *testing.T) {
	// Test that the working directory is successfully set to whatever was passed in.
	dir, err := ioutil.TempDir("", "exec-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	e, err := New(
		`C:\Windows\System32\cmd.exe`,
		"cmd /c echo 'test' > test.txt",
		WithDir(dir),
		WithEnv(os.Environ()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	err = e.Wait()
	if err != nil {
		t.Fatalf("error waiting for process: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "test.txt")); err != nil {
		t.Fatal(err)
	}

	t.Logf("exit code was: %d", e.ExitCode())
}

func TestExecStdinPowershell(t *testing.T) {
	// Exec a powershell instance and test that we can write commands to stdin and receive the output from stdout.
	e, err := New(
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		"powershell",
		WithStdio(true, false, true),
		WithEnv(os.Environ()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	errChan := make(chan error)
	go func() {
		_, _ = io.Copy(os.Stdout, e.Stdout())
	}()

	go func() {
		cmd := "ping 127.0.0.1\r\n"
		if _, err := e.Stdin().Write([]byte(cmd)); err != nil {
			errChan <- err
		}

		exit := "exit\r\n"
		if _, err := e.Stdin().Write([]byte(exit)); err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}

	err = e.Wait()
	if err != nil {
		t.Fatalf("error waiting for process: %v", err)
	}
	t.Logf("exit code was: %d", e.ExitCode())
}

func TestExecWithJob(t *testing.T) {
	// Test that we can assign a process to a job object at creation time.
	job, err := jobobject.Create(context.Background(), &jobobject.Options{Name: "test"})
	if err != nil {
		log.Fatal(err)
	}
	defer job.Close()

	e, err := New(
		`C:\Windows\System32\ping.exe`,
		"ping 127.0.0.1",
		WithJobObject(job),
		WithStdio(true, false, false),
		WithEnv(os.Environ()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pids, err := job.Pids()
	if err != nil {
		t.Fatal(err)
	}

	if len(pids) == 0 {
		t.Fatal("no pids found in job object")
	}

	// Should only be one process in the job, being the process we launched and added to it.
	if pids[0] != uint32(e.Pid()) {
		t.Fatal(err)
	}

	err = e.Wait()
	if err != nil {
		t.Fatalf("error waiting for process: %v", err)
	}
	t.Logf("exit code was: %d", e.ExitCode())
}

func TestPseudoConsolePowershell(t *testing.T) {
	cpty, err := conpty.New(80, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cpty.Close()

	// Exec a powershell instance and test that we can write commands to the input side of the pty and receive data
	// from the output end.
	e, err := New(
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		"powershell",
		WithEnv(os.Environ()),
		WithConPty(cpty),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	errChan := make(chan error)
	go func() {
		buf := make([]byte, 1000)
		for {
			_, err := cpty.Read(buf)
			if err != nil {
				errChan <- err
			}

			if !strings.Contains(string(buf), "howdy from conpty") {
				continue
			}
			close(errChan)
			break
		}
	}()

	cmd := "echo \"howdy from conpty\"\r\n"
	if _, err := cpty.Write([]byte(cmd)); err != nil {
		t.Fatal(err)
	}

	// If after the timeout we haven't read the output we wrote to the pseudo console below then
	// fail the test.
	select {
	case <-time.After(time.Second * 10):
		t.Fatal("timed out waiting for output to pseudo console")
	case err := <-errChan:
		if err != nil {
			t.Fatal(err)
		}
	}

	exit := "exit\r\n"
	if _, err := cpty.Write([]byte(exit)); err != nil {
		t.Fatal(err)
	}

	err = e.Wait()
	if err != nil {
		t.Fatalf("error waiting for process: %v", err)
	}
	t.Logf("exit code was: %d", e.ExitCode())
}
