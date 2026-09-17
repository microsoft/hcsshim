//go:build windows

package wclayer

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	winio "github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// errWriter always fails writes, used to simulate a full disk (ENOSPC) when the
// buffered writer is flushed.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("simulated: there is not enough space on the disk")
}

// Test_legacyLayerWriter_reset_ClosesFileOnFlushError verifies that reset closes
// the current file handle even when bufWriter.Flush fails (e.g. the disk is
// full). Before the fix, an error from Flush returned early and left the file
// handle open, which on Windows prevented the temporary import directory
// (C:\Windows\SystemTemp\hcs*) from being removed, leaking it.
func Test_legacyLayerWriter_reset_ClosesFileOnFlushError(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "current")
	f, err := os.Create(fpath)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	w := &legacyLayerWriter{
		currentFile: f,
		// bufWriter wraps a writer that always fails so Flush returns an error.
		bufWriter: bufio.NewWriter(errWriter{}),
	}
	// Buffer some data so Flush actually attempts a (failing) write.
	if _, err := w.bufWriter.WriteString("data"); err != nil {
		t.Fatalf("failed to buffer data: %v", err)
	}

	err = w.reset()
	if err == nil {
		// Close the handle to avoid leaking it if the expectation is not met.
		f.Close()
		t.Fatal("expected reset to return the flush error, got nil")
	}

	if w.currentFile != nil {
		t.Error("expected currentFile to be nil after reset, handle was not closed")
	}

	// On Windows an open *os.File cannot be removed (no FILE_SHARE_DELETE), so a
	// successful remove proves the handle was actually released by reset.
	if err := os.Remove(fpath); err != nil {
		t.Errorf("expected temp file to be removable after reset (handle closed), got: %v", err)
	}
}

// Test_legacyLayerWriter_reset_ClearsCurrentIsDirOnError verifies that
// currentIsDir is cleared by the deferred cleanup when an error occurs inside
// the currentIsDir handling block. Before the fix, currentIsDir remained true
// after the defer closed currentFile, so the next call to reset would enter the
// currentIsDir block and panic with a nil pointer dereference on r.Seek.
func Test_legacyLayerWriter_reset_ClearsCurrentIsDirOnError(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "current")
	f, err := os.Create(fpath)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Write non-zero content so Flush succeeds but the backup stream Seek
	// returns a file-level error (empty file means Seek succeeds but the
	// backup-stream reader immediately sees EOF before returning an error from
	// the reparse-data path; writing garbage bytes causes winio to return an
	// error from Next()).
	if _, err := f.WriteString("not-a-valid-backup-stream"); err != nil {
		f.Close()
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		t.Fatalf("failed to seek temp file: %v", err)
	}

	w := &legacyLayerWriter{
		currentFile:  f,
		currentIsDir: true,
		bufWriter:    bufio.NewWriter(io.Discard),
	}

	// reset should return an error from inside the currentIsDir block.
	if err := w.reset(); err == nil {
		t.Fatal("expected reset to return an error from the currentIsDir block, got nil")
	}

	// The critical invariant: currentIsDir must be cleared by the deferred
	// cleanup so that a subsequent call to reset does not panic.
	if w.currentIsDir {
		t.Error("expected currentIsDir to be false after reset error, got true")
	}
	if w.currentFile != nil {
		t.Error("expected currentFile to be nil after reset error")
	}

	// A second call to reset must not panic (nil-deref on r.Seek was the bug).
	w.bufWriter = bufio.NewWriter(io.Discard)
	if err := w.reset(); err != nil {
		t.Errorf("second reset call returned unexpected error: %v", err)
	}
}

// Test_legacyLayerWriter_reset_DirFlagWithoutFile verifies reset is robust when
// currentIsDir is true but currentFile is nil (a state that can occur if an
// earlier error path closed/cleared currentFile before currentIsDir was reset).
func Test_legacyLayerWriter_reset_DirFlagWithoutFile(t *testing.T) {
	w := &legacyLayerWriter{
		currentIsDir: true,
		bufWriter:    bufio.NewWriter(io.Discard),
	}

	if err := w.reset(); err != nil {
		t.Fatalf("expected reset to succeed when currentFile is nil, got: %v", err)
	}
	if w.currentIsDir {
		t.Error("expected currentIsDir to be false after reset")
	}
}

// closes and clears the current file handle.
func Test_legacyLayerWriter_reset_ClosesFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "current")
	f, err := os.Create(fpath)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	w := &legacyLayerWriter{
		currentFile: f,
		bufWriter:   bufio.NewWriter(io.Discard),
	}

	if err := w.reset(); err != nil {
		f.Close()
		t.Fatalf("expected reset to succeed, got: %v", err)
	}

	if w.currentFile != nil {
		t.Error("expected currentFile to be nil after reset")
	}
	if w.currentFileName != "" || w.currentFileRoot != nil {
		t.Error("expected currentFileName/currentFileRoot to be cleared after reset")
	}

	if err := os.Remove(fpath); err != nil {
		t.Errorf("expected temp file to be removable after reset (handle closed), got: %v", err)
	}
}

func setSparse(t *testing.T, f *os.File) {
	t.Helper()
	if err := windows.DeviceIoControl(windows.Handle(f.Fd()), windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, nil, nil); err != nil {
		t.Fatalf("set sparse: %v", err)
	}
}

func TestFindBackupStreamSizeSparse(t *testing.T) {
	//nolint:gosec // G306: test files do not need restrictive permissions
	for name, setup := range map[string]func(*testing.T) string{
		"normalFile": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo")
			if err := os.WriteFile(path, []byte("testing 1 2 3\n"), 0644); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"normalFileEmpty": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			return path
		},
		"sparseEmpty": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			return path
		},
		"sparseAllHoles": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if err := f.Truncate(1048576); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"sparseOneRange": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if _, err := f.WriteString("test sparse data"); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"sparseMultipleRanges": func(t *testing.T) string {
			t.Helper()
			path := filepath.Join(t.TempDir(), "foo")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			setSparse(t, f)
			if _, err = f.Write([]byte("leading data\n")); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Seek(1048576, 0); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write([]byte("trailing data\n")); err != nil {
				t.Fatal(err)
			}
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := setup(t)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			fi, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}

			br := winio.NewBackupFileReader(f, true)
			defer br.Close()

			size, err := findBackupStreamSize(br)
			if err != nil {
				t.Fatalf("findBackupStreamSize: %v", err)
			}
			if size != fi.Size() {
				t.Errorf("findBackupStreamSize = %d, want logical size %d", size, fi.Size())
			}
		})
	}
}
