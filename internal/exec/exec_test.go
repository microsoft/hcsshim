package exec

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
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
	dir := t.TempDir()

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

	waitChan := make(chan error)
	go func() {
		waitChan <- e.Wait()
	}()

	select {
	case err := <-waitChan:
		if err != nil {
			t.Fatalf("error waiting for process: %v", err)
		}
	case <-time.After(time.Second * 10):
		_ = e.Kill()
		t.Fatal("timed out waiting for process to complete")
	}

	t.Logf("exit code was: %d", e.ExitCode())
}

func TestExecsWithJob(t *testing.T) {
	// Test that we can assign processes to a job object at creation time.
	job, err := jobobject.Create(context.Background(), &jobobject.Options{Name: "test"})
	if err != nil {
		log.Fatal(err)
	}
	defer job.Close()

	e, err := New(
		`C:\Windows\System32\ping.exe`,
		"ping -t 127.0.0.1",
		WithJobObject(job),
		WithStdio(false, false, false),
		WithEnv(os.Environ()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	// Launch a second process and check pids.
	e2, err := New(
		`C:\Windows\System32\ping.exe`,
		"ping -t 127.0.0.1",
		WithJobObject(job),
		WithStdio(false, false, false),
		WithEnv(os.Environ()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = e2.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pidMap := map[int]struct{}{
		e.Pid():  {},
		e2.Pid(): {},
	}

	pids, err := job.Pids()
	if err != nil {
		t.Fatal(err)
	}

	if len(pids) != 2 {
		t.Fatalf("should be two pids in job object, got: %d", len(pids))
	}

	for _, pid := range pids {
		if _, ok := pidMap[int(pid)]; !ok {
			t.Fatalf("failed to find pid %d in job object", pid)
		}
	}

	err = e.Kill()
	if err != nil {
		t.Fatalf("error killing process: %v", err)
	}

	err = e2.Kill()
	if err != nil {
		t.Fatalf("error killing process: %v", err)
	}

	_ = e.Wait()
	_ = e2.Wait()

	if !e.Exited() {
		t.Fatalf("Process %d should have exited after kill", e.Pid())
	}
	if !e2.Exited() {
		t.Fatalf("Process %d should have exited after kill", e2.Pid())
	}
}

func TestPseudoConsolePowershell(t *testing.T) {
	// This test is fairly flaky on the Github CI but seems to run fine locally. Skip this for now to let contributions continue without a hitch
	// and until we can replace this with a better suited test shortly.
	//
	// TODO(dcantah): Fix/find a better test here
	t.Skip("Skipping flaky test")
	cpty, err := conpty.Create(80, 20, 0)
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
		_, _ = io.Copy(os.Stdout, cpty.OutPipe())
	}()

	go func() {
		cmd := "ping 127.0.0.1\r\n"
		if _, err := cpty.Write([]byte(cmd)); err != nil {
			errChan <- err
		}

		exit := "exit\r\n"
		if _, err := cpty.Write([]byte(exit)); err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}

	waitChan := make(chan error)
	go func() {
		waitChan <- e.Wait()
	}()

	select {
	case err := <-waitChan:
		if err != nil {
			t.Fatalf("error waiting for process: %v", err)
		}
	case <-time.After(time.Second * 10):
		_ = e.Kill()
		t.Fatal("timed out waiting for process to complete")
	}

	t.Logf("exit code was: %d", e.ExitCode())
}
