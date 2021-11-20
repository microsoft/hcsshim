package exec

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

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
	// Exec a powershel instance and test that we can write commands to stdin and receive the output from stdout.
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

	stdinChan := make(chan error)
	go func() {
		_, _ = io.Copy(os.Stdout, e.Stdout())
	}()

	go func() {
		cmd := `ping 127.0.0.1
		`

		exit := `exit
		`
		if _, err := e.Stdin().Write([]byte(cmd)); err != nil {
			stdinChan <- err
		}
		if _, err := e.Stdin().Write([]byte(exit)); err != nil {
			stdinChan <- err
		}
		close(stdinChan)
	}()

	err = <-stdinChan
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

	// Should only be one process in the job
	if pids[0] != uint32(e.Pid()) {
		t.Fatal(err)
	}

	err = e.Wait()
	if err != nil {
		t.Fatalf("error waiting for process: %v", err)
	}
	t.Logf("exit code was: %d", e.ExitCode())
}
