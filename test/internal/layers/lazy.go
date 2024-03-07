//go:build windows

package layers

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/hcsshim/ext4/dmverity"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/security"
	isync "github.com/Microsoft/hcsshim/internal/sync"
	"github.com/Microsoft/hcsshim/pkg/ociwclayer"

	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/images"
)

// helper utilities for dealing with images

// TODO: create a `type innerImageLayers struct{ dir, layers }` and use `testsync.Once[innerImageLayers]` instead

type LazyImageLayers struct {
	Image        string
	Platform     string
	AppendVerity bool
	// TempPath is the path to create a temporary directory in.
	// Defaults to [os.TempDir] if left empty.
	TempPath string
	// dedicated directory, under [TempPath], to store layers in
	dir    string
	once   sync.Once
	layers []string // extracted layer directories, under [dir]
}

type extractHandler func(ctx context.Context, rc io.ReadCloser, dir string, parents []string) error

// Close removes the downloaded image layers.
//
// Does not take a [testing.TB] so it can be used in TestMain or init.
func (x *LazyImageLayers) Close(ctx context.Context) error {
	if x == nil || x.dir == "" {
		return nil
	}

	log.G(ctx).WithFields(logrus.Fields{
		"dir":   x.dir,
		"image": x.Image,
	}).Debug("removing image")

	if _, err := os.Stat(x.dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("path %q is not valid: %w", x.dir, err)
	}

	// DestroyLayer will remove the entire directory and all its contents, regardless of if
	// its a Windows container layer or not.
	if err := util.DestroyLayer(ctx, x.dir); err != nil {
		return fmt.Errorf("could not destroy layer directory %q: %w", x.dir, err)
	}
	return nil
}

// Layers returns the image layer paths, from lowest to highest, for a particular image.
func (x *LazyImageLayers) Layers(ctx context.Context, tb testing.TB) []string {
	// basically combo of containerd fetch and unpack (snapshotter + differ)
	tb.Helper()

	if x == nil {
		return nil
	}

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
	if x.Image == "" {
		return fmt.Errorf("cannot return layers for an empty image")
	}

	if x.TempPath == "" {
		dir, err := tempDirOnce(ctx)
		if err != nil {
			return err
		}
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

	log.G(ctx).WithFields(logrus.Fields{
		"platform": x.Platform,
		"image":    x.Image,
		"path":     x.dir,
	}).Info("pulling and unpacking image layers")

	extract, err := extractImageHandler(x.Platform, x.AppendVerity)
	if err != nil {
		return err
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

func extractImageHandler(platform string, appendVerity bool) (extractHandler, error) {
	var extract extractHandler
	if platform == images.PlatformLinux {
		extract = linuxExt4LayerExtractHandler()
		if appendVerity {
			extract = withAppendVerity(extract)
		}
		extract = withVHDFooter(extract)
		return extract, nil
	} else if platform == images.PlatformWindows {
		return windowsImage, nil
	}
	return nil, fmt.Errorf("unsupported platform %q", platform)
}

func linuxExt4LayerExtractHandler() extractHandler {
	return func(_ context.Context, rc io.ReadCloser, dir string, _ []string) error {
		f, err := os.Create(filepath.Join(dir, "layer.vhd"))
		if err != nil {
			return fmt.Errorf("create layer vhd: %w", err)
		}
		defer f.Close()

		convertOpts := []tar2ext4.Option{
			tar2ext4.ConvertWhiteout,
			tar2ext4.MaximumDiskSize(dmverity.RecommendedVHDSizeGB),
		}
		if err := tar2ext4.Convert(rc, f, convertOpts...); err != nil {
			return fmt.Errorf("convert to ext4 %s: %w", f.Name(), err)
		}
		if err := f.Sync(); err != nil {
			return fmt.Errorf("sync ext4 file %s to disk: %w", f.Name(), err)
		}
		return nil
	}
}

func withAppendVerity(fn extractHandler) extractHandler {
	return func(ctx context.Context, rc io.ReadCloser, dir string, parents []string) error {
		if err := fn(ctx, rc, dir, parents); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(dir, "layer.vhd"), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return fmt.Errorf("unable to prepare file to append verity: %w", err)
		}

		if err := dmverity.ComputeAndWriteHashDevice(f, f); err != nil {
			return fmt.Errorf("unable to compute and append hash device: %w", err)
		}
		return nil
	}
}

func withVHDFooter(fn extractHandler) extractHandler {
	return func(ctx context.Context, rc io.ReadCloser, dir string, parents []string) error {
		if err := fn(ctx, rc, dir, parents); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(dir, "layer.vhd"), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer f.Close()

		if err := tar2ext4.ConvertToVhd(f); err != nil {
			return fmt.Errorf("unable to convert file to VHD: %w", err)
		}

		if err = security.GrantVmGroupAccess(f.Name()); err != nil {
			return fmt.Errorf("grant vm group access to %s: %w", f.Name(), err)
		}
		return nil
	}
}

var procPrivilegesOnce = isync.OnceValueCtx(func(ctx context.Context) (struct{}, error) {
	privs := []string{winio.SeBackupPrivilege, winio.SeRestorePrivilege}
	log.G(ctx).WithField("privileges", privs).Infof("enableing process privileges")

	return struct{}{}, winio.EnableProcessPrivileges(privs)
})

func windowsImage(ctx context.Context, rc io.ReadCloser, dir string, parents []string) error {
	if _, err := procPrivilegesOnce(ctx); err != nil {
		return fmt.Errorf("enable process Backup and Restore privileges: %w", err)
	}

	if _, err := ociwclayer.ImportLayerFromTar(ctx, rc, dir, parents); err != nil {
		return fmt.Errorf("import wc layer %s: %w", dir, err)
	}

	return nil
}

// tempDirOnce returns a dedicated folder in [os.TempDir] that is used for all image layers
// (that don't specify their [TempDir] field), as well as scratch and cache VHDs.
//
// The directory is excluded from MS Defender, and persisted between test runs to reduce
// the overhead associated with unpacking images and creating uVMs.
//
// Since the folder is under TempDir, it will be removed on OS restart, so it is fine to persist
// between test runs
var tempDirOnce = isync.OnceValueCtx(func(ctx context.Context) (string, error) {
	// ! DO NOT DELETE THE FOLDER CREATED BY THIS FUNC:
	// We want a "stable" (relative to OS restart) directory for image layers and
	// scratch files that we can avoid needing to recreate and re-adding defender exclusions to.

	dir := filepath.Join(os.TempDir(), "hcsshim-test")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create hcsshim testing temp directory: %w", err)
	}

	cmd := exec.Command("powershell.exe", "-NoLogo", "-NonInteractive", "-Command",
		"Add-MpPreference -ExclusionPath '"+dir+"'")
	o, err := cmd.CombinedOutput()
	if err != nil {
		// not necessary to creating the image layers path, so log then ignore error
		log.G(ctx).WithFields(logrus.Fields{
			"cmd":    cmd.String(),
			"output": strings.TrimSpace(string(o)),
			"path":   dir,
		}).WithError(err).Warning("failed to add MS defender exclusion for image layers directory")
	} else {
		log.G(ctx).WithField("path", dir).Info("added MS Defender exclusion for image layers directory")
	}

	return dir, nil
})
