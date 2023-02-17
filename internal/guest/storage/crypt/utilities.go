//go:build linux
// +build linux

package crypt

import (
	"context"
	"crypto/rand"
	"io"
	"os"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/pkg/errors"
)

// getBlockDeviceSize returns the size of the specified block device.
func getBlockDeviceSize(ctx context.Context, path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, errors.Wrap(err, "error opening: "+path)
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.G(ctx).WithError(err).Debug("error closing: " + path)
		}
	}()

	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, errors.Wrap(err, "error seeking end of: "+path)
	}

	return pos, nil
}


// In the xfs mkfs case it appears to attempt to read the first block of the device.
// This results in an integrity error. This function zeros out the start of the device
// so we are sure that when it is read it has already been hashed so matches.

func zeroDevice(devicePath string, blockSize int64, numberOfBlocks int64) error {
	fout, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to open device file %s", devicePath)
	}
	defer fout.Close()

	zeros := make([]byte, blockSize)
	for i := range zeros {
		zeros[i] = 0
	}

	// get the size so we don't overrun the end of the device
	foutSize, err := fout.Seek(0, io.SeekEnd)
	if err != nil {
		return errors.Wrapf(err, "zeroDevice: failed to seek to end, device file %s", devicePath)
	}

	// move back to the front.
	_, err = fout.Seek(0, io.SeekStart)
	if err != nil {
		return errors.Wrapf(err, "zeroDevice: failed to seek to start, device file %s", devicePath)
	}

	var offset int64 = 0
	var which int64
	for which = 0; which < numberOfBlocks; which++ {
		// Exit when the end of the file is reached
		if offset >= foutSize {
			break
		}
		// Write data to destination file
		wrttten, err := fout.Write(zeros)
		if err != nil {
			return errors.Wrapf(err, "failed to write destination file %s offset %d", devicePath, offset)
		}
		offset += int64(wrttten)
	}

	return nil
}

// generateKeyFile generates a file with random values.
func generateKeyFile(path string, size int64) error {
	// The crypto.rand interface generates random numbers using /dev/urandom
	keyArray := make([]byte, size)
	_, err := rand.Read(keyArray[:])
	if err != nil {
		return errors.Wrap(err, "failed to generate key array")
	}

	if err := os.WriteFile(path, keyArray[:], 0644); err != nil {
		return errors.Wrap(err, "failed to save key to file")
	}

	return nil
}
