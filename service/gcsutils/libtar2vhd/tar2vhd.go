package libtar2vhd

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/docker/docker/pkg/archive"

	"github.com/Microsoft/opengcs/service/gcsutils/fs"
	"github.com/Microsoft/opengcs/service/gcsutils/tarlib"
	"github.com/Microsoft/opengcs/service/gcsutils/vhd"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
)

type Options struct {
	TarOpts       *archive.TarOptions
	Filesystem    fs.Filesystem
	Converter     vhd.Converter
	TempDirectory string
}

func Tar2VHD(in io.Reader, out io.Writer, options *Options) (int64, error) {
	utils.LogMsg("creating a temp file for VHD")

	// Create a VHD file
	vhdFile, err := ioutil.TempFile(options.TempDirectory, "vhd")
	if err != nil {
		return 0, err
	}

	defer os.Remove(vhdFile.Name())
	defer vhdFile.Close()

	utils.LogMsg("create Tar disk")
	// Write Tar file to vhd
	if _, err := tarlib.CreateTarDisk(in,
		options.Filesystem,
		options.TarOpts,
		options.TempDirectory,
		vhdFile); err != nil {
		return 0, err
	}

	utils.LogMsg("convert to VHD")
	if err := options.Converter.ConvertToVHD(vhdFile); err != nil {
		return 0, err
	}

	utils.LogMsg("send to std out pipe")
	diskSize, err := io.Copy(out, vhdFile)
	if err != nil {
		return 0, err
	}
	utils.LogMsgf("leaving Tar2VHD: VHD disk size:%d", diskSize)
	return diskSize, nil
}

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

	if err := exec.Command("mount", "-o", "loop", vhdFile.Name(), mntFolder).Run(); err != nil {
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

func VHDX2Tar(mntPath string, out io.Writer, options *Options) (int64, error) {
	// The actual files are located in <mnt_path>/upper
	readerResult, err := archive.TarWithOptions(filepath.Join(mntPath, "upper"), options.TarOpts)
	if err != nil {
		return 0, err
	}

	retSize, err := io.Copy(out, readerResult)
	if err != nil {
		return 0, err
	}
	return retSize, nil
}
