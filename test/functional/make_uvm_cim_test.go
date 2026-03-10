//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Microsoft/go-winio/pkg/fs"
	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/Microsoft/hcsshim/pkg/cimfs"
	"github.com/Microsoft/hcsshim/pkg/extractuvm"
)

func compareFiles(t *testing.T, file1, file2 string) (bool, error) {
	t.Helper()

	file1, err := fs.ResolvePath(file1)
	if err != nil {
		return false, err
	}

	file2, err = fs.ResolvePath(file2)
	if err != nil {
		return false, err
	}

	// Get file info for both files
	info1, err := os.Stat(file1)
	if err != nil {
		return false, err
	}
	info2, err := os.Stat(file2)
	if err != nil {
		return false, err
	}

	// Only compare file sizes for now
	return info1.Size() == info2.Size(), nil
}

func compareDirs(t *testing.T, dir1, dir2 string) {
	t.Helper()

	var differences []string
	var onlyInDir1 []string
	var onlyInDir2 []string

	// Walk through the first directory
	err := filepath.Walk(dir1, func(path1 string, info1 os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Construct the corresponding path in the second directory
		relativePath := strings.TrimPrefix(path1, dir1)
		path2 := filepath.Join(dir2, relativePath)

		_, err = os.Stat(path2)
		if os.IsNotExist(err) {
			onlyInDir1 = append(onlyInDir1, relativePath)
			return nil
		} else if err != nil {
			return err
		}

		// If the file exists in both directories, compare it
		if info1.IsDir() {
			// Skip directories, we don't compare their contents here
			return nil
		}

		isIdentical, err := compareFiles(t, path1, path2)
		if err != nil {
			return err
		}
		if !isIdentical {
			differences = append(differences, relativePath)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("comparison failed: %s", err)
	}

	// Walk through the second directory to find files that are not in the first directory
	err = filepath.Walk(dir2, func(path2 string, info2 os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Construct the corresponding path in the first directory
		relativePath := strings.TrimPrefix(path2, dir2)
		path1 := filepath.Join(dir1, relativePath)

		_, err = os.Stat(path1)
		if os.IsNotExist(err) {
			onlyInDir2 = append(onlyInDir2, relativePath)
		} else if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		t.Fatalf("comparison failed: %s", err)
	}

	failed := false
	// Output results
	if len(differences) > 0 {
		failed = true
		t.Log("Files that differ in size between the directories:")
		for _, diff := range differences {
			t.Log(diff)
		}
	}

	if len(onlyInDir1) > 0 {
		failed = true
		t.Logf("Files/directories only in %s:", dir1)
		for _, item := range onlyInDir1 {
			t.Log(item)
		}
	}

	if len(onlyInDir2) > 0 {
		failed = true
		t.Logf("Files/directories only in %s:", dir2)
		for _, item := range onlyInDir2 {
			t.Log(item)
		}
	}

	if failed {
		t.Fatalf("directories not identical!")
	}
}

func saveLayerTar(t *testing.T, layer v1.Layer, savePath string) {
	t.Helper()

	// Open a file to write the layer content
	outputFile, err := os.Create(savePath)
	if err != nil {
		t.Fatalf("creating tarball file %s: %s", savePath, err)
	}
	defer outputFile.Close()

	// Get a reader for the layer
	layerReader, err := layer.Compressed()
	if err != nil {
		t.Fatalf("getting reader for layer: %s", err)
	}
	defer layerReader.Close()

	// Copy the layer content to the file
	if _, err := io.Copy(outputFile, layerReader); err != nil {
		t.Fatalf("failed to write layer to file: %s", err)
	}
}

func mountBlockCIM(t *testing.T, bCIM *cimfs.BlockCIM, mountFlags uint32) string {
	t.Helper()
	// mount and read the contents of the cim
	volumeGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("generate cim mount GUID: %s", err)
	}

	mountvol, err := cimfs.Mount(filepath.Join(bCIM.BlockPath, bCIM.CimName), volumeGUID, mountFlags)
	if err != nil {
		t.Fatalf("mount cim : %s", err)
	}
	t.Cleanup(func() {
		if err := cimfs.Unmount(mountvol); err != nil {
			t.Logf("CIM unmount failed: %s", err)
		}
	})
	return mountvol
}

// TestCompareUtilityVMCIM compares generated UtilityVM CIM's contents against the WCIFS based layer and ensures that they match.
func TestCompareUtilityVMCIM(t *testing.T) {
	if !cimfs.IsBlockCimMountSupported() {
		t.Skip("block CIMs are not supported on this build")
	}

	// extract image locally using default WCIFS based layers
	nanoserverLayers := windowsImageLayers(context.TODO(), t)

	// download the nanoserver base layer tarball
	wcowImages, err := wcowImagePathsOnce()
	if err != nil {
		t.Fatalf("failed to get wcow images: %s", err)
	}

	img, err := crane.Pull(wcowImages.nanoserver.Image, crane.WithPlatform(&v1.Platform{
		OS: wcowImages.nanoserver.Platform, Architecture: runtime.GOARCH,
	}))
	if err != nil {
		t.Fatalf("failed to pull nanoserver image: %s", err)
	}

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("failed to get image layers: %s", err)
	}

	layerTarPath := filepath.Join(t.TempDir(), "nanoserver_base.tar")
	// nanoserver has only 1 layer
	saveLayerTar(t, layers[0], layerTarPath)

	// use the tarball to extract out the UtilityVM layer
	extractPath := t.TempDir()
	uvmCIM, err := extractuvm.MakeUtilityVMCIMFromTar(context.Background(), layerTarPath, extractPath)
	if err != nil {
		t.Fatalf("failed to make UtilityVM CIM: %s", err)
	}

	mountvol := mountBlockCIM(t, uvmCIM, cimfs.CimMountSingleFileCim)

	compareDirs(t, filepath.Join(nanoserverLayers[0], "UtilityVM\\Files"), mountvol)

}
