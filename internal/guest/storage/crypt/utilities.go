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

// createSparseEmptyFile creates a sparse file of the specified size. The whole
// file is empty, so the size on disk is zero, only the logical size is the
// specified one.
func createSparseEmptyFile(ctx context.Context, path string, size int64) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "failed to create: %s", path)
	}

	defer func() {
		if err != nil {
			if inErr := os.RemoveAll(path); inErr != nil {
				log.G(ctx).WithError(inErr).Debug("failed to delete: " + path)
			}
		}
	}()

	defer func() {
		if err := f.Close(); err != nil {
			log.G(ctx).WithError(err).Debug("failed to close: " + path)
		}
	}()

	if err := f.Truncate(size); err != nil {
		return errors.Wrapf(err, "failed to truncate: %s", path)
	}

	return nil
}

// The following constants aren't defined in the io or os libraries.
//
//nolint:stylecheck // ST1003: ALL_CAPS
const (
	SEEK_DATA = 3
	SEEK_HOLE = 4
)

// copyEmptySparseFilesystem copies data chunks of a sparse source file into a
// destination file. It skips holes. Note that this is intended to copy a
// filesystem that has just been generated, so it only contains metadata blocks.
// Because of that, the source file must end with a hole. If it ends with data,
// the last chunk of data won't be copied.
func copyEmptySparseFilesystem(source string, destination string) error {
	fin, err := os.OpenFile(source, os.O_RDONLY, 0)
	if err != nil {
		return errors.Wrap(err, "failed to open source file")
	}
	defer fin.Close()

	fout, err := os.OpenFile(destination, os.O_WRONLY, 0)
	if err != nil {
		return errors.Wrap(err, "failed to open destination file")
	}
	defer fout.Close()

	finInfo, err := fin.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to stat source file")
	}

	finSize := finInfo.Size()

	var offset int64 = 0
	for {
		// Exit when the end of the file is reached
		if offset >= finSize {
			break
		}

		// Calculate bounds of the next data chunk
		chunkStart, err := fin.Seek(offset, SEEK_DATA)
		if (err != nil) || (chunkStart == -1) {
			// No more chunks left
			break
		}
		chunkEnd, err := fin.Seek(chunkStart, SEEK_HOLE)
		if (err != nil) || (chunkEnd == -1) {
			break
		}
		chunkSize := chunkEnd - chunkStart
		offset = chunkEnd

		// Read contents of this data chunk
		//nolint:staticcheck //TODO: SA1019: os.SEEK_SET has been deprecated since Go 1.7: Use io.SeekStart, io.SeekCurrent, and io.SeekEnd.
		_, err = fin.Seek(chunkStart, os.SEEK_SET)
		if err != nil {
			return errors.Wrap(err, "failed to seek set in source file")
		}

		chunkData := make([]byte, chunkSize)
		count, err := fin.Read(chunkData)
		if err != nil {
			return errors.Wrap(err, "failed to read source file")
		}
		if int64(count) != chunkSize {
			return errors.Wrap(err, "not enough data read from source file")
		}

		// Write data to destination file
		//nolint:staticcheck //TODO: SA1019: os.SEEK_SET has been deprecated since Go 1.7: Use io.SeekStart, io.SeekCurrent, and io.SeekEnd.
		_, err = fout.Seek(chunkStart, os.SEEK_SET)
		if err != nil {
			return errors.Wrap(err, "failed to seek destination file")
		}
		_, err = fout.Write(chunkData)
		if err != nil {
			return errors.Wrap(err, "failed to write destination file")
		}
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
