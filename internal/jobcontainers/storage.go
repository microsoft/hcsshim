package jobcontainers

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

func mountLayers(ctx context.Context, containerId string, s *specs.Spec) (string, error) {
	if s == nil || s.Windows == nil || s.Windows.LayerFolders == nil {
		return "", errors.New("field 'Spec.Windows.Layerfolders' is not populated")
	}

	// Last layer always contains the sandbox.vhdx, or 'scratch' space for the container.
	scratchFolder := s.Windows.LayerFolders[len(s.Windows.LayerFolders)-1]
	if _, err := os.Stat(scratchFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(scratchFolder, 0777); err != nil {
			return "", errors.Wrapf(err, "failed to auto-create container scratch folder %s", scratchFolder)
		}
	}

	// Create sandbox.vhdx if it doesn't exist in the scratch folder.
	if _, err := os.Stat(filepath.Join(scratchFolder, "sandbox.vhdx")); os.IsNotExist(err) {
		if err := wclayer.CreateScratchLayer(ctx, scratchFolder, s.Windows.LayerFolders[:len(s.Windows.LayerFolders)-1]); err != nil {
			return "", errors.Wrap(err, "failed to CreateSandboxLayer")
		}
	}

	if s.Root == nil {
		s.Root = &specs.Root{}
	}

	// Make a temp directory to mount the volume (for now). Can't figure out how to pass volume path to any of the
	// go stdlib io methods. Might just have to call win32 calls directly (also need to figure this out :) ).
	tempDir, err := ioutil.TempDir("", containerId)
	if err != nil {
		return "", errors.Wrap(err, "failed to create a temp directory for job container volume")
	}
	// SetVolumeMountPoint requires a trailing slash.
	tempDir += "\\"

	log.G(ctx).Debug("mounting job container storage")
	containerRootPath, err := layers.MountContainerLayers(ctx, containerId, s.Windows.LayerFolders, "", tempDir, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to mount job container storage")
	}
	s.Root.Path = containerRootPath

	return tempDir, nil
}

// setupBindings sets up the file system view for the container. This is done by using a filesystem minifilter that allows us to bind
// a file system namespace to another location. The filter allows per silo bindings, so only the silo that you made the mapping for would be
// viewable by processes in the silo. Processes on the host, or processes in another silo would not see anything bound for a specific silo.
//
// The steps we take are to loop through every top level file and directory in the image and then bind each one to the root of the C drive.
// The files in the image are merged together with files on the host, and if any files conflict (same name) the files from the image will take precedence
// and shadow the files on the host. As we're still using the same copy on write scratch volume based mechanism as we do for Windows Server Containers,
// any files or directories that are from the container image and are unique (no directory with the same name on the host), we'll still get copy on write for
// those files.
//
// For now, we skip binding the Windows directory (if there is one) as there's some wonky behavior with loading certain programs (powershell for one).
// The Bind Filter is essentially analogous to a bind mount on Linux, and this is also how directory mounts are handled for Windows Server Containers as well.
func (c *JobContainer) setupBindings(volumeMountPath string) error {
	files, err := ioutil.ReadDir(volumeMountPath)
	if err != nil {
		return errors.Wrap(err, "failed to root of job containers volume")
	}

	for _, file := range files {
		// Skip the Windows dir for now. Also skip the wcsandboxstate dir that is usually a hidden folder.
		if file.Name() == "Windows" || file.Name() == "WcSandboxState" {
			continue
		}
		// This could also potentially be the entire C volume against the entire volume of the container image,
		// but I'll need to see how exclusions work.
		if err := c.job.ApplyFileBinding(
			filepath.Join("C:\\", file.Name()),
			filepath.Join(volumeMountPath, file.Name()),
			true,
		); err != nil {
			return err
		}
	}
	return nil
}
