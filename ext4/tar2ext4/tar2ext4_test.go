package tar2ext4

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"archive/tar"
	"os"
	"strings"
	"time"
)

// Test_UnorderedTarExpansion tests that we are correctly able to expand a layer tar file
// which has one or many files in an unordered fashion. By unordered we mean that the
// entry of a file shows up during an expansion before the entry of one of the parent
// directories of that file.  In such cases we create that parent directory with same
// permissions as its parent and then later on fix the permissions when we actually see
// the entry of that parent directory.
func Test_UnorderedTarExpansion(t *testing.T) {
	tmpTarFilePath := filepath.Join(os.TempDir(), "test-layer.tar")
	layerTar, err := os.Create(tmpTarFilePath)
	if err != nil {
		t.Fatalf("failed to create output file: %s", err)
	}
	defer os.Remove(tmpTarFilePath)

	tw := tar.NewWriter(layerTar)
	var files = []struct {
		path, body string
	}{
		{"foo/.wh.bar.txt", "inside bar.txt"},
		{"data/", ""},
		{"root.txt", "inside root.txt"},
		{"foo/", ""},
		{"A/.wh..wh..opq", ""},
		{"A/B/b.txt", "inside b.txt"},
		{"A/a.txt", "inside a.txt"},
		{"A/", ""},
		{"A/B/", ""},
	}
	for _, file := range files {
		var hdr *tar.Header
		if strings.HasSuffix(file.path, "/") {
			hdr = &tar.Header{
				Name:       file.path,
				Mode:       0777,
				Size:       0,
				ModTime:    time.Now(),
				AccessTime: time.Now(),
				ChangeTime: time.Now(),
			}
		} else {
			hdr = &tar.Header{
				Name:       file.path,
				Mode:       0777,
				Size:       int64(len(file.body)),
				ModTime:    time.Now(),
				AccessTime: time.Now(),
				ChangeTime: time.Now(),
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(file.path, "/") {
			if _, err := tw.Write([]byte(file.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Now try to import this tar and verify that there is no failure.
	if _, err := layerTar.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %s", err)
	}

	opts := []Option{AppendVhdFooter, ConvertWhiteout}
	tmpVhdPath := filepath.Join(os.TempDir(), "test-vhd.vhdx")
	layerVhd, err := os.Create(tmpVhdPath)
	if err != nil {
		t.Fatalf("failed to create output VHD: %s", err)
	}
	defer os.Remove(tmpVhdPath)

	if err := Convert(layerTar, layerVhd, opts...); err != nil {
		t.Fatalf("failed to convert tar to layer vhd: %s", err)
	}
}

func Test_TarHardlinkToSymlink(t *testing.T) {
	tmpTarFilePath := filepath.Join(os.TempDir(), "test-layer.tar")
	layerTar, err := os.Create(tmpTarFilePath)
	if err != nil {
		t.Fatalf("failed to create output file: %s", err)
	}
	defer os.Remove(tmpTarFilePath)

	tw := tar.NewWriter(layerTar)

	var files = []struct {
		path     string
		typeFlag byte
		linkName string
		body     string
	}{
		{
			path: "/tmp/zzz.txt",
			body: "inside /tmp/zzz.txt",
		},
		{
			path:     "/tmp/xxx.txt",
			linkName: "/tmp/zzz.txt",
			typeFlag: tar.TypeSymlink,
		},
		{
			path:     "/tmp/yyy.txt",
			linkName: "/tmp/xxx.txt",
			typeFlag: tar.TypeLink,
		},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name:       file.path,
			Typeflag:   file.typeFlag,
			Linkname:   file.linkName,
			Mode:       0777,
			Size:       int64(len(file.body)),
			ModTime:    time.Now(),
			AccessTime: time.Now(),
			ChangeTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if file.body != "" {
			if _, err := tw.Write([]byte(file.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Now try to import this tar and verify that there is no failure.
	if _, err := layerTar.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %s", err)
	}

	opts := []Option{AppendVhdFooter, ConvertWhiteout}
	tmpVhdPath := filepath.Join(os.TempDir(), "test-vhd.vhdx")
	layerVhd, err := os.Create(tmpVhdPath)
	if err != nil {
		t.Fatalf("failed to create output VHD: %s", err)
	}
	defer os.Remove(tmpVhdPath)

	if err := Convert(layerTar, layerVhd, opts...); err != nil {
		t.Fatalf("failed to convert tar to layer vhd: %s", err)
	}
}

// Test_MissingParentDirExpansion tests that we are correctly able to expand a layer tar file
// even if its file does not include the parent directory in its file name.
func Test_MissingParentDirExpansion(t *testing.T) {
	tmpTarFilePath := filepath.Join(os.TempDir(), "test-layer.tar")
	layerTar, err := os.Create(tmpTarFilePath)
	if err != nil {
		t.Fatalf("failed to create output file: %s", err)
	}
	defer os.Remove(tmpTarFilePath)

	tw := tar.NewWriter(layerTar)
	var files = []struct {
		path, body string
	}{
		{"foo/bar.txt", "inside bar.txt"},
	}
	for _, file := range files {
		var hdr *tar.Header
		if strings.HasSuffix(file.path, "/") {
			hdr = &tar.Header{
				Name:       file.path,
				Mode:       0777,
				Size:       0,
				ModTime:    time.Now(),
				AccessTime: time.Now(),
				ChangeTime: time.Now(),
			}
		} else {
			hdr = &tar.Header{
				Name:       file.path,
				Mode:       0777,
				Size:       int64(len(file.body)),
				ModTime:    time.Now(),
				AccessTime: time.Now(),
				ChangeTime: time.Now(),
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(file.path, "/") {
			if _, err := tw.Write([]byte(file.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Now import the tar file and check the conversion to ext4 is deterministic.
	if _, err := layerTar.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %s", err)
	}

	opts := []Option{ConvertWhiteout}

	tmpVhdPath1 := filepath.Join(os.TempDir(), "test-vhd1.ext4")
	layerVhd1, err := os.Create(tmpVhdPath1)
	if err != nil {
		t.Fatalf("failed to create output VHD: %s", err)
	}
	defer os.Remove(tmpVhdPath1)

	if err := Convert(layerTar, layerVhd1, opts...); err != nil {
		t.Fatalf("failed to convert tar to layer vhd: %s", err)
	}

	// Check the conversion is deterministic.
	tmpVhdPath2 := filepath.Join(os.TempDir(), "test-vhd2.ext4")
	layerVhd2, err := os.Create(tmpVhdPath2)
	if err != nil {
		t.Fatalf("failed to create output VHD: %s", err)
	}
	defer os.Remove(tmpVhdPath2)

	if _, err := layerTar.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %s", err)
	}

	if err := Convert(layerTar, layerVhd2, opts...); err != nil {
		t.Fatalf("failed to convert tar to layer vhd: %s", err)
	}

	if _, err := layerVhd1.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %s", err)
	}

	if _, err := layerVhd2.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %s", err)
	}

	hasher1 := sha256.New()
	if _, err = io.Copy(hasher1, layerVhd1); err != nil {
		t.Fatalf("filed to initialize hasher: %s", err)
	}

	hash1 := hasher1.Sum(nil)
	hash1hex := fmt.Sprintf("%x", hash1)

	hasher2 := sha256.New()
	if _, err = io.Copy(hasher2, layerVhd2); err != nil {
		t.Fatalf("filed to initialize hasher: %s", err)
	}
	hash2 := hasher2.Sum(nil)
	hash2hex := fmt.Sprintf("%x", hash2)

	if hash1hex != hash2hex {
		t.Fatalf("hash doesn't match")
	}
}
