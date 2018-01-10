package libtar2vhd

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/archive"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/opengcs/service/gcsutils/fs"
	"github.com/Microsoft/opengcs/service/gcsutils/tarlib"
	"github.com/Microsoft/opengcs/service/gcsutils/vhd"
)

// Options contains the configuration parameters that get passed to the tar2vhd library.
type Options struct {
	TarOpts       *archive.TarOptions // Docker's archive.TarOptions struct
	Filesystem    fs.Filesystem       // Interface for type of filesystem
	Converter     vhd.Converter       // Interface for type of whiteout file
	TempDirectory string              // Temp directory used for the conversions
}

// Tar2VHD takes in a tarstream and outputs a vhd containing the files. It also
// returns the size of the outputted VHD file.
func Tar2VHD(in io.Reader, out io.Writer, options *Options) (int64, error) {
	logrus.Info("creating a temp file for VHD")

	// Create a VHD file
	vhdFile, err := ioutil.TempFile(options.TempDirectory, "vhd")
	if err != nil {
		return 0, err
	}

	defer os.Remove(vhdFile.Name())
	defer vhdFile.Close()

	logrus.Info("create Tar disk")
	// Write Tar file to vhd
	if _, err := tarlib.CreateTarDisk(in,
		options.Filesystem,
		options.TarOpts,
		options.TempDirectory,
		vhdFile); err != nil {
		return 0, err
	}

	logrus.Info("convert to VHD")
	if err := options.Converter.ConvertToVHD(vhdFile); err != nil {
		return 0, err
	}

	logrus.Info("send to std out pipe")
	diskSize, err := io.Copy(out, vhdFile)
	if err != nil {
		return 0, err
	}
	logrus.Infof("leaving Tar2VHD: VHD disk size:%d", diskSize)
	return diskSize, nil
}

// VHD2Tar takes in a vhd and outputs a tar stream containing the files in the
// vhd. It also returns the size of the tar stream.
func VHD2Tar(in io.Reader, out io.Writer, options *Options) (int64, error) {
	// First write the VHD to disk. We want random access for some vhd operations
	vhdFile, err := ioutil.TempFile(options.TempDirectory, "vhd")
	if err != nil {
		return 0, err
	}
	defer os.Remove(vhdFile.Name())
	defer vhdFile.Close()

	if _, err := io.Copy(vhdFile, in); err != nil {
		return 0, err
	}

	if err := options.Converter.ConvertFromVHD(vhdFile); err != nil {
		return 0, err
	}

	mntFolder, err := ioutil.TempDir("", "mnt")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(mntFolder)

	if err := exec.Command("mount", "-t", "ext4", "-o", "loop", vhdFile.Name(), mntFolder).Run(); err != nil {
		return 0, err
	}
	defer exec.Command("umount", mntFolder).Run()

	readerResult, err := archive.TarWithOptions(mntFolder, options.TarOpts)
	if err != nil {
		return 0, err
	}

	tarSize, err := io.Copy(out, readerResult)
	if err != nil {
		return 0, err
	}
	return tarSize, nil
}

// VHDX2Tar takes in a folder (can be mounted from an attached VHDX) and returns a tar stream
// containing the contents of the folder. It also returns the size of the tar stream.
func VHDX2Tar(mntPath string, out io.Writer, options *Options) (int64, error) {
	// If using overlay, the actual files are located in <mnt_path>/upper.
	// `FROM SCRATCH` uses a regular ext4 mount.
	logrus.Infof("VHDX2Tar on mount path %s", mntPath)
	pm, err := os.Open("/proc/mounts")
	if err != nil {
		e := fmt.Errorf("failed to open /proc/mounts %s", err)
		logrus.Errorf(e.Error())
		return 0, e
	}
	defer pm.Close()
	scanner := bufio.NewScanner(pm)
	overlay := true
	for scanner.Scan() {
		logrus.Infof("scanning mount line %q", scanner.Text())
		if strings.Contains(scanner.Text(), mntPath) {
			logrus.Info("mount line contains the mount path")
			if !strings.Contains(scanner.Text(), "overlay") {
				logrus.Info("mount line does not contain overlay, but still could be overlay - looking for 'upper'")
				s, err := os.Stat(filepath.Join(mntPath, "upper"))
				if err != nil {
					if os.IsNotExist(err) {
						logrus.Info("'upper' does not exist, so definitely not overlay")
						overlay = false
						break
					}
					e := fmt.Errorf("failed to stat %s: %s", filepath.Join(mntPath, "upper"), err)
					logrus.Errorf(e.Error())
					return 0, e
				}
				if !s.IsDir() {
					logrus.Info("'upper' is not a directory, so not overlay")
					overlay = false
				}
			}
			// Exit loop as found the line which has the mount path
			break
		}
	}
	if overlay {
		mntPath = filepath.Join(mntPath, "upper")
		logrus.Infof("is overlay so updated mount path to %s", mntPath)
	}

	readerResult, err := archive.TarWithOptions(mntPath, options.TarOpts)
	if err != nil {
		e := fmt.Errorf("failed to TarWithOptions %s (%+v): %s", mntPath, options.TarOpts, err)
		logrus.Errorf(e.Error())
		return 0, err
	}

	retSize, err := io.Copy(out, readerResult)
	if err != nil {
		e := fmt.Errorf("failed to io.Copy the tar stream back to the caller: %s", err)
		logrus.Errorf(e.Error())
		return 0, err
	}
	logrus.Infof("copied %d bytes of tarstream back", retSize)
	return retSize, nil
}
