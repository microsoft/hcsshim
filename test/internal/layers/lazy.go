//go:build windows

package layers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/security"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/pkg/ociwclayer"

	"github.com/Microsoft/hcsshim/test/internal/constants"
	"github.com/Microsoft/hcsshim/test/internal/util"
)

// helper utilities for dealing with images

type LazyImageLayers struct {
	Image    string
	Platform string
	// TempPath is the path to create a temporary directory in.
	// Defaults to [os.TempDir] if left empty.
	TempPath string
	// dedicated directory, under [TempPath], to store layers in
	dir    string
	once   sync.Once
	layers []string
}

// Close removes the downloaded image layers.
//
// Does not take a [testing.TB] so it can be used in TestMain or init.
func (x *LazyImageLayers) Close(ctx context.Context) error {
	if x.dir == "" {
		return nil
	}

	if _, err := os.Stat(x.dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("path %q is not valid: %w", x.dir, err)
	}
	// DestroyLayer will remove the entire directory and all its contents, regardless of if
	// its a Windows container layer or not.
	if err := wclayer.DestroyLayer(ctx, x.dir); err != nil {
		return fmt.Errorf("could not destroy layer directory %q: %w", x.dir, err)
	}
	return nil
}

// Layers returns the image layer paths, from lowest to highest, for a particular image.
func (x *LazyImageLayers) Layers(ctx context.Context, tb testing.TB) []string {
	// basically combo of containerd fetch and unpack (snapshotter + differ)
	tb.Helper()
	var err error
	x.once.Do(func() {
		err = x.extractLayers(ctx)
	})
	if err != nil {
		x.Close(ctx)
		tb.Fatal(err)
	}
	return x.layers
}

// don't use tb.Error/Log inside Once.Do stack, since we cannot call tb.Helper before executing f()
// within Once.Do and that will therefore show the wrong stack/location
func (x *LazyImageLayers) extractLayers(ctx context.Context) (err error) {
	log.G(ctx).Infof("pulling and unpacking %s image %q", x.Platform, x.Image)

	if x.TempPath == "" {
		dir := os.TempDir()
		x.dir, err = os.MkdirTemp(dir, util.CleanName(x.Image))
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
	} else {
		x.dir, err = filepath.Abs(x.TempPath)
		if err != nil {
			return fmt.Errorf("failed to make %q absolute path: %w", x.TempPath, err)
		}
	}

	var extract func(context.Context, io.ReadCloser, string, []string) error
	switch x.Platform {
	case constants.PlatformLinux:
		extract = linuxImage
	case constants.PlatformWindows:
		if err = winio.EnableProcessPrivileges([]string{winio.SeBackupPrivilege, winio.SeRestorePrivilege}); err != nil {
			return err
		}
		extract = windowsImage
	default:
		return fmt.Errorf("unsupported platform %q", x.Platform)
	}

	img, err := crane.Pull(x.Image, crane.WithPlatform(&v1.Platform{OS: x.Platform, Architecture: runtime.GOARCH}))
	if err != nil {
		return fmt.Errorf("failed to pull image %q: %w", x.Image, err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("failed to get image %q layers: %w", x.Image, err)
	}

	for i, l := range layers {
		d := filepath.Join(x.dir, strconv.FormatInt(int64(i), 10))
		if err := os.Mkdir(d, 0755); err != nil {
			return err
		}
		rc, err := l.Uncompressed()
		if err != nil {
			return fmt.Errorf("failed to load uncompressed layer for image %s: %w", x.Image, err)
		}
		defer rc.Close()
		if err := extract(ctx, rc, d, x.layers); err != nil {
			return fmt.Errorf("failed to extract layer %d for image %s: %w", i, x.Image, err)
		}
		x.layers = append(x.layers, d)
	}

	return nil
}

func linuxImage(ctx context.Context, rc io.ReadCloser, dir string, _ []string) error {
	f, err := os.Create(filepath.Join(dir, "layer.vhd"))
	if err != nil {
		return fmt.Errorf("create layer vhd: %w", err)
	}
	// in case we fail before granting access; double close on file will no-op
	defer f.Close()

	if err := tar2ext4.Convert(rc, f, tar2ext4.AppendVhdFooter, tar2ext4.ConvertWhiteout); err != nil {
		return fmt.Errorf("convert to vhd %s: %w", f.Name(), err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync vhd %s to disk: %w", f.Name(), err)
	}
	f.Close()

	if err = security.GrantVmGroupAccess(f.Name()); err != nil {
		return fmt.Errorf("grant vm group access to %s: %w", f.Name(), err)
	}

	return nil
}

func windowsImage(ctx context.Context, rc io.ReadCloser, dir string, parents []string) error {
	if _, err := ociwclayer.ImportLayerFromTar(ctx, rc, dir, parents); err != nil {
		return fmt.Errorf("import wc layer %s: %w", dir, err)
	}

	return nil
}
