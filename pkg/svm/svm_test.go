package svm

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

// TestNewInvalidOptions ensures that calls to New with invalid options fail.
func TestNewInvalidOptions(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := New(&NewOptions{Mode: 0}); err == nil {
		t.Fatal("expected failure")
	}
}

func createTempDirs(t *testing.T) (string, string) {
	scratchDir, err := ioutil.TempDir("", "scratchDir")
	if err != nil {
		t.Fatalf("failed to create tempdir: %s", err)
	}
	cacheDir, err := ioutil.TempDir("", "cacheDir")
	if err != nil {
		t.Fatalf("failed to create tempdir: %s", err)
	}
	return cacheDir, scratchDir
}

// TestCreateDestroyGlobal creates a single global SVM in an instance and tears
// it down using destroy.
func TestCreateDestroyGlobal(t *testing.T) {
	i := newInstance(t, ModeGlobal)

	cacheDir, scratchDir := createTempDirs(t)
	defer os.RemoveAll(cacheDir)
	defer os.RemoveAll(scratchDir)

	create(t, i, "some-irrelevant-id", cacheDir, scratchDir)

	if err := i.Destroy(); err != nil {
		t.Fatal("should succeed")
	}
	checkZeroComputeSystems(t)
	checkCount(t, i, 0)

}

// TestCreateDestroyUniqueTwoSVMs creates two SVMs in an instance and tears them
// down using destroy. It validates that the cached scratch and scratches are
// created and mounted
func TestCreateDestroyUniqueTwoSVMs(t *testing.T) {
	i := newInstance(t, ModeUnique)
	cacheDir, scratchDir := createTempDirs(t)
	defer os.RemoveAll(cacheDir)
	defer os.RemoveAll(scratchDir)
	create(t, i, "first", cacheDir, scratchDir)
	checkCount(t, i, 1)
	create(t, i, "second", cacheDir, scratchDir)
	checkCount(t, i, 2)
	checkFileExists(t, filepath.Join(cacheDir, "cache_ext4.20GB.vhdx"))
	checkFileExists(t, filepath.Join(scratchDir, "first_svm_scratch.vhdx"))
	checkFileExists(t, filepath.Join(scratchDir, "second_svm_scratch.vhdx"))

	var out bytes.Buffer
	po := &ProcessOptions{
		Id:          "first",
		Args:        []string{"mount"},
		Stdout:      &out,
		CopyTimeout: 30 * time.Second,
	}
	_, ec, err := i.RunProcess(po)
	if err != nil {
		t.Fatalf("expected success: %s", err)
	}
	if ec != 0 {
		t.Fatalf("expected zero exit code: %d", ec)
	}
	if !strings.Contains(out.String(), "/dev/sda on /tmp/scratch type ext4") {
		t.Fatalf("expected to find '/dev/sda on /tmp/scratch type ext4': %s", out.String())
	}
	if err := i.Destroy(); err != nil {
		t.Fatal("should succeed")
	}
	checkZeroComputeSystems(t)
	checkCount(t, i, 0)
}

// TestDestroyNilMap makes sure destroy works even if we never created/started a service VM
func TestDestroyNilMap(t *testing.T) {
	i := newInstance(t, ModeGlobal)
	if err := i.Destroy(); err != nil {
		t.Fatal("should succeed")
	}
	checkCount(t, i, 0)
}

// TestMode validates the Mode() method on an instance
func TestMode(t *testing.T) {
	gi := newInstance(t, ModeGlobal)
	ui := newInstance(t, ModeUnique)
	if gi.Mode() != ModeGlobal || ui.Mode() != ModeUnique {
		t.Fatal("returned incorrect mode")
	}
	checkCount(t, gi, 0)
	checkCount(t, ui, 0)
}

// TestDiscardGlobal ensures that discard in global mode succeeds
func TestDiscardGlobal(t *testing.T) {
	gi := newInstance(t, ModeGlobal)
	if gi.Discard("anything") != nil {
		t.Fatal("should succeed")
	}
	checkZeroComputeSystems(t)
	checkCount(t, gi, 0)
}

// TestDiscardUnique verifies discard in unique mode succeeds if an ID
// is valid, and fails when an ID is invalid.
func TestDiscardUnique(t *testing.T) {
	i := newInstance(t, ModeUnique)
	id := "TestDiscardUniqueDoesntExist"
	cacheDir, scratchDir := createTempDirs(t)
	defer os.RemoveAll(cacheDir)
	defer os.RemoveAll(scratchDir)
	create(t, i, id, cacheDir, scratchDir)
	checkCount(t, i, 1)
	if err := i.Discard("does not exist"); err != ErrNotFound {
		t.Fatalf("expected %s, got %s", ErrNotFound, err)
	}
	if err := i.Discard(id); err != nil {
		t.Fatal("expected success")
	}
	checkZeroComputeSystems(t)
	checkCount(t, i, 0)
}

// Helper to remove contents of a directory
func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

// Helper to validate a directory is empty
func checkEmptyDir(t *testing.T, name string) {
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("expected %s to be empty", name)
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return
	}
	t.Fatalf("expected %s to be empty", name)
}

// Helper to validate a file exists
func checkFileExists(t *testing.T, name string) {
	if _, err := os.Stat(name); err == nil {
		return
	}
	t.Fatalf("expected %s to exist", name)
}

func newInstance(t *testing.T, mode Mode) Instance {
	i, err := New(&NewOptions{Mode: mode})
	if err != nil {
		t.Fatal("should succeed")
	}
	return i
}

func create(t *testing.T, i Instance, id string, cacheDir string, scratchDir string) {
	if err := i.Create(id, cacheDir, scratchDir); err != nil {
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
