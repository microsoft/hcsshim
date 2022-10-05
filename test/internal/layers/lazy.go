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
	"golang.org/x/sync/errgroup"

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
	TempPath string // TempPath is the path to create a temporary directory in. Default in [os.TempDir]
	once     sync.Once
	layers   []string
}

// ImageLayers returns the image layer paths, from lowest to highest, for a particular image.
func (x *LazyImageLayers) ImageLayers(ctx context.Context, tb testing.TB) []string {
	tb.Helper()
	tb.Logf("pulling and unpacking %s image %q", x.Platform, x.Image)
	// don't use tb.Error/Log inside Once.Do stack, since we cannot call tb.Helper before executing f()
	// within Once.Do and that will therefore show the wrong stack/location
	var err error
	x.once.Do(func() {
		var dir string
		// tb.TempDir is deleted at the end of a test, but we want the image for future test runs
		dir, err = os.MkdirTemp(x.TempPath, util.CleanName(x.Image))
		if err != nil {
			err = fmt.Errorf("failed to create temp directory: %w", err)
			return
		}

		switch x.Platform {
		case constants.PlatformLinux:
			err = x.linuxImage(ctx, dir)
		case constants.PlatformWindows:
			err = x.windowsImage(ctx, dir)
		default:
			err = fmt.Errorf("unsupported platform %q", x.Platform)
		}
	})
	if err != nil {
		x.Close(ctx)
		tb.Fatal(err)
	}
	return x.layers
}

// Close removes the downloaded image layers.
func (x *LazyImageLayers) Close(ctx context.Context) {
	for _, dir := range x.layers {
		d, err := filepath.Abs(dir)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("count not get absolute path to %q", dir)
			continue
		}
		if _, err := os.Stat(d); err != nil {
			log.G(ctx).WithError(err).Errorf("path %q is not valid", d)
			continue
		}
		if err := wclayer.DestroyLayer(ctx, d); err != nil {
			log.G(ctx).WithError(err).Errorf("could not destroy layer %q", d)
		}
	}
}

func (x *LazyImageLayers) linuxImage(ctx context.Context, dir string) error {
	img, err := crane.Pull(x.Image, crane.WithPlatform(&v1.Platform{OS: x.Platform, Architecture: runtime.GOARCH}))
	if err != nil {
		return fmt.Errorf("failed to pull %q: %w", x.Image, err)
	}

	f, err := os.Create(filepath.Join(dir, "layer.vhd"))
	if err != nil {
		return fmt.Errorf("failed to create layer vhd: %w", err)
	}
	defer f.Close()
	// update x.layers so x.close() does the right thing if this function fails
	x.layers = []string{dir}

	r, w := io.Pipe()
	eg := errgroup.Group{}
	eg.Go(func() error {
		defer w.Close()
		if err := crane.Export(img, w); err != nil {
			return fmt.Errorf("export image %q: %w", x.Image, err)
		}
		return nil
	})

	eg.Go(func() error {
		defer r.Close()
		if err := tar2ext4.Convert(r, f, tar2ext4.AppendVhdFooter, tar2ext4.ConvertWhiteout); err != nil {
			return fmt.Errorf("convert image %q to vhd %q: %w", x.Image, f.Name(), err)
		}
		if err := f.Sync(); err != nil {
			return fmt.Errorf("sync vhd %q to disk: %w", f.Name(), err)
		}
		f.Close()
		if err = security.GrantVmGroupAccess(f.Name()); err != nil {
			return fmt.Errorf("grant vm group access to %s: %w", f.Name(), err)
		}
		return nil
	})

	return eg.Wait()
}

func (x *LazyImageLayers) windowsImage(ctx context.Context, dir string) error {
	img, err := crane.Pull(x.Image, crane.WithPlatform(&v1.Platform{OS: x.Platform, Architecture: runtime.GOARCH}))
	if err != nil {
		return fmt.Errorf("failed to pull %q: %w", x.Image, err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("failed to get image %q layers: %w", x.Image, err)
	}
	if err := winio.EnableProcessPrivileges([]string{winio.SeBackupPrivilege, winio.SeRestorePrivilege}); err != nil {
		return fmt.Errorf("could not set process privileges: %w", err)
	}

	for i, l := range layers {
		d := filepath.Join(dir, strconv.FormatInt(int64(i), 10))
		if err := os.Mkdir(d, 0755); err != nil {
			return err
		}
		rc, err := l.Uncompressed()
		if err != nil {
			return fmt.Errorf("failed to load uncompressed layer: %w", err)
		}
		if _, err := ociwclayer.ImportLayerFromTar(ctx, rc, d, x.layers); err != nil {
			return fmt.Errorf("failed to import wc layer %d: %w", i, err)
		}

		x.layers = append(x.layers, d)
	}
	return nil
}
