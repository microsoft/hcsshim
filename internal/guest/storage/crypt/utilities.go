//go:build linux
// +build linux

package crypt

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"github.com/Microsoft/hcsshim/internal/log"
)

// getBlockDeviceSize returns the size of the specified block device.
func getBlockDeviceSize(ctx context.Context, path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("error opening %s: %w", path, err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.G(ctx).WithError(err).Debug("error closing: " + path)
		}
	}()

	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("error seeking end of %s: %w", path, err)
	}
	return pos, nil
}

func zeroFirstBlock(path string, blockSize int) error {
	fout, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open file for zero'ing: %w", err)
	}
	defer fout.Close()

	zeros := bytes.Repeat([]byte{0}, blockSize)
	if _, err := fout.Write(zeros); err != nil {
		return fmt.Errorf("failed writing zero bytes: %w", err)
	}
	return nil
}

// generateKeyFile generates a file with random values.
func generateKeyFile(path string, size int64) error {
	// The crypto.rand interface generates random numbers using /dev/urandom
	keyArray := make([]byte, size)
	_, err := rand.Read(keyArray[:])
	if err != nil {
		return fmt.Errorf("failed to generate key slice: %w", err)
	}

	if err := os.WriteFile(path, keyArray[:], 0644); err != nil {
		return fmt.Errorf("failed to save key to file: %w", err)
	}
	return nil
}
