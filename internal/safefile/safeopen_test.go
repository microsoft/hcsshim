package safefile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	winio "github.com/Microsoft/go-winio"
)

func tempRoot(t *testing.T) (*os.File, error) {
	name := t.TempDir()
	f, err := OpenRoot(name)
	if err != nil {
		return nil, err
	}

	t.Cleanup(func() {
		_ = f.Close()
	})

	return f, err
}

func TestRemoveRelativeReadOnly(t *testing.T) {
	root, err := tempRoot(t)
	if err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(root.Name(), "foo")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	bi := winio.FileBasicInfo{}
	bi.FileAttributes = syscall.FILE_ATTRIBUTE_READONLY
	err = winio.SetFileBasicInfo(f, &bi)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	err = RemoveRelative("foo", root)
	if err != nil {
		t.Fatal(err)
	}
}
