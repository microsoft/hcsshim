package svm

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// // TestNewInvalidOptions ensures that calls to New with invalid options fail.
// func TestNewInvalidOptions(t *testing.T) {
// 	if _, err := New(nil); err == nil {
// 		t.Fatal("expected failure")
// 	}
// 	if _, err := New(&NewOptions{Mode: 0}); err == nil {
// 		t.Fatal("expected failure")
// 	}
// }

// // TestCreateDestroyGlobal creates a single global SVM in an instance and tears
// // it down using destroy.
// func TestCreateDestroyGlobal(t *testing.T) {
// 	i := newInstance(t, ModeGlobal)
// 	create(t, i, "some-irrelevant-id")
// 	if err := i.Destroy(); err != nil {
// 		t.Fatal("should succeed")
// 	}
// 	checkZeroComputeSystems(t)
// 	checkCount(t, i, 0)
// }

// // TestCreateDestroyUniqueTwoSVMs creates two SVMs in an instance and tears them
// // down using destroy.
// func TestCreateDestroyUniqueTwoSVMs(t *testing.T) {
// 	i := newInstance(t, ModeUnique)
// 	create(t, i, "first")
// 	checkCount(t, i, 1)
// 	create(t, i, "second")
// 	checkCount(t, i, 2)
// 	if err := i.Destroy(); err != nil {
// 		t.Fatal("should succeed")
// 	}
// 	checkZeroComputeSystems(t)
// 	checkCount(t, i, 0)
// }

// // TestDestroyNilMap makes sure destroy works even if we never created/started a service VM
// func TestDestroyNilMap(t *testing.T) {
// 	i := newInstance(t, ModeGlobal)
// 	if err := i.Destroy(); err != nil {
// 		t.Fatal("should succeed")
// 	}
// 	checkCount(t, i, 0)
// }

// // TestMode validates the Mode() method on an instance
// func TestMode(t *testing.T) {
// 	gi := newInstance(t, ModeGlobal)
// 	ui := newInstance(t, ModeUnique)
// 	if gi.Mode() != ModeGlobal || ui.Mode() != ModeUnique {
// 		t.Fatal("returned incorrect mode")
// 	}
// 	checkCount(t, gi, 0)
// 	checkCount(t, ui, 0)
// }

// // TestDiscardGlobal ensures that discard in global mode succeeds
// func TestDiscardGlobal(t *testing.T) {
// 	gi := newInstance(t, ModeGlobal)
// 	if gi.Discard("anything") != nil {
// 		t.Fatal("should succeed")
// 	}
// 	checkZeroComputeSystems(t)
// 	checkCount(t, gi, 0)
// }

// // TestDiscardUnique verifies discard in unique mode succeeds if an ID
// // is valid, and fails when an ID is invalid.
// func TestDiscardUnique(t *testing.T) {
// 	i := newInstance(t, ModeUnique)
// 	id := "TestDiscardUniqueDoesntExist"
// 	create(t, i, id)
// 	checkCount(t, i, 1)
// 	if err := i.Discard("does not exist"); err != ErrNotFound {
// 		t.Fatalf("expected %s, got %s", ErrNotFound, err)
// 	}
// 	if err := i.Discard(id); err != nil {
// 		t.Fatal("expected success")
// 	}
// 	checkZeroComputeSystems(t)
// 	checkCount(t, i, 0)
// }

// TestProcess tests launching processes in a service VM
func TestProcess(t *testing.T) {
	ig := newInstance(t, ModeGlobal)
	defer ig.Destroy()
	create(t, ig, "dont-care-as-global")
	testProcess(t, ig)

	ug := newInstance(t, ModeUnique)
	defer ug.Destroy()
	create(t, ug, "anything")
	testProcess(t, ug)
}

func testProcess(t *testing.T, i Instance) {
	// A process which succeeds, check it's output
	ec, output, err := i.RunProcess("anything", []string{"ls", "-l", "/"}, "")
	if ec != 0 || err != nil {
		t.Fatalf("expected success ec=%d err=%d", ec, err)
	}
	if !strings.Contains(output, "lost+found") {
		t.Fatalf("output was %s", output)
	}

	// A non-zero exit code
	ec, output, err = i.RunProcess("anything", []string{"sh", "-c", `"exit 123"`}, "")
	if err != nil {
		t.Fatalf("err was %s", err)
	}
	if ec != 123 {
		t.Fatalf("ec was %d", ec)
	}

	// Command not found
	ec, output, err = i.RunProcess("anything", []string{"foobarbaz"}, "")
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "executable file not found in $PATH") {
		t.Fatalf("didn't find what we were looking for %s", err)
	}

	// Something to stderr is returned in output
	ec, output, err = i.RunProcess("anything", []string{"cat", "some-file-which-does-not-exist"}, "")
	if err != nil {
		t.Fatalf("expected success")
	}
	if !strings.Contains(output, "cat: can't open 'some-file-which-does-not-exist': No such file or directory") {
		t.Fatalf("unexpected output %s", output)
	}
	if ec != 1 {
		t.Fatalf("ec was %d", ec)
	}

	// Send stdin
	ec, output, err = i.RunProcess("anything", []string{"cat < /proc/self/fd/0"}, "hello\r\nworld\r\n")
	if err != nil {
		t.Fatalf("expected success")
	}
	fmt.Println(output)
}

func newInstance(t *testing.T, mode Mode) Instance {
	i, err := New(&NewOptions{Mode: mode})
	if err != nil {
		t.Fatal("should succeed")
	}
	return i
}

func create(t *testing.T, i Instance, id string) {
	if err := i.Create(id); err != nil {
		t.Fatalf("failed create %s: %s", id, err)
	}
}

func checkZeroComputeSystems(t *testing.T) {
	cmd := exec.Command("powershell", "-command", "$(get-computeprocess).Length")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	sout := strings.Replace(string(out), "\r\n", "", -1)
	if sout != "0" {
		t.Fatalf("expected 0 compute systems, there are '%s' running", sout)
	}
}

func checkCount(t *testing.T, i Instance, expected int) {
	c := i.Count()
	if c != expected {
		t.Fatalf("expected %d, have %d service VMs", expected, c)
	}
}
