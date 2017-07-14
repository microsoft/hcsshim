package tarlib

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path"

	"github.com/Microsoft/opengcs/service/gcsutils/fs"
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/archive"
)

func createEmptyDisk(in io.Reader,
	out io.Writer,
	f fs.Filesystem,
	options *archive.TarOptions,
	disk *os.File) (uint64, error) {

	logrus.Info("entering createEmptyDisk")

	inTar := tar.NewReader(in)
	outTar := tar.NewWriter(out)
	defer outTar.Close()

	// First, determine the size of the tar file.
	logrus.Info("determine the size of the tar file")

	if err := f.InitSizeContext(); err != nil {
		return 0, err
	}

	var totalBytesRecieved int64

	totalBytesRecieved = 0

	for {
		hdr, err := inTar.Next()
		if err == io.EOF {
			logrus.Info("EOF file read")
			break
		} else if err != nil {
			return 0, err
		}

		// Handle whiteouts
		isWhiteout, err := CalcWhiteoutSize(hdr, f, options.WhiteoutFormat)
		if err != nil {
			return 0, err
		}

		// Handle real files
		if !isWhiteout {
			switch hdr.Typeflag {
			case tar.TypeDir:
				err = f.CalcDirSize(hdr.Name)
			case tar.TypeReg, tar.TypeRegA:
				err = f.CalcRegFileSize(hdr.Name, uint64(hdr.Size))
			case tar.TypeLink:
				err = f.CalcHardlinkSize(hdr.Linkname, hdr.Name)
			case tar.TypeSymlink:
				err = f.CalcSymlinkSize(hdr.Linkname, hdr.Name)
			case tar.TypeBlock:
				err = f.CalcBlockDeviceSize(hdr.Name, 0, 0)
			case tar.TypeChar:
				err = f.CalcCharDeviceSize(hdr.Name, 0, 0)
			case tar.TypeFifo:
				err = f.CalcFIFOPipeSize(hdr.Name)
			default:
				err = fmt.Errorf("unknown tar type %d", hdr.Typeflag)
			}

			if err != nil {
				return 0, err
			}
		}

		// Write header to write buffer since we want to keep the tar
		if err := outTar.WriteHeader(hdr); err != nil {
			return 0, err
		}
		var bytesWritten int64
		if bytesWritten, err = io.Copy(outTar, inTar); err != nil {
			return 0, err
		}
		totalBytesRecieved += bytesWritten
	}
	logrus.Infof("totalBytesRecieved = %d", totalBytesRecieved)
	if err := f.FinalizeSizeContext(); err != nil {
		return 0, err
	}

	sizeInfo := f.GetSizeInfo()
	if sizeInfo.TotalSize > math.MaxInt64 {
		return 0, fmt.Errorf("tar file too big: %d", sizeInfo.TotalSize)
	}

	// Now, create the file system.
	if err := disk.Truncate(int64(sizeInfo.TotalSize)); err != nil {
		return 0, err
	}

	if err := f.MakeFileSystem(disk); err != nil {
		logrus.Infof("f.MakeFileSystem failed with %s", err)
		return 0, err
	}

	if err := f.CleanupSizeContext(); err != nil {
		return 0, err
	}
	return sizeInfo.TotalSize, nil
}

// CreateTarDisk creates a file system image from the input tarstream with the
// given parameters and writes the image to the output file. It also returns
// the size of the image.
func CreateTarDisk(in io.Reader,
	f fs.Filesystem,
	options *archive.TarOptions,
	tmpdir string,
	disk *os.File) (uint64, error) {

	logrus.Info("entering CreateTarDisk")

	mntFolder, err := ioutil.TempDir(tmpdir, "mnt")
	if err != nil {
		return 0, err
	}

	defer os.RemoveAll(mntFolder)

	tmpFile, err := ioutil.TempFile(tmpdir, "tempTar")
	if err != nil {
		return 0, err
	}

	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	diskSize, err := createEmptyDisk(in, tmpFile, f, options, disk)
	if err != nil {
		logrus.Infof("calling createEmptyDisk failed with %s", err)
		return 0, err
	}

	// Mount the disk and remove the lost+found folder that might appear from mkfs
	if err := exec.Command("mount", "-o", "loop", disk.Name(), mntFolder).Run(); err != nil {
		logrus.Infof("failed mount -o loop %s", err)
		return 0, err
	}
	defer exec.Command("umount", mntFolder).Run()

	if err := os.RemoveAll(path.Join(mntFolder, "lost+found")); err != nil {
		// RemoveAll doesn't return error on missing file, so we don't need to special case it.
		return 0, err
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		return 0, err
	}

	if err := archive.Unpack(tmpFile, mntFolder, options); err != nil {
		return 0, err
	}
	return diskSize, nil
}
