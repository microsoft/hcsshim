//go:build linux
// +build linux

package crypt

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
)

func zeroFirstBlock(path string, blockSize int) error {
	fout, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open file for zero'ing: %w", err)
	}
	defer fout.Close()

	size, err := fout.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("error seeking end of %s: %w", path, err)
	}
	if size < int64(blockSize) {
		return fmt.Errorf("file size is smaller than minimum expected: %d < %d", size, blockSize)
	}

	_, err = fout.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("error seeking start of %s: %w", path, err)
	}

	zeros := bytes.Repeat([]byte{0}, blockSize)
	if _, err := fout.Write(zeros); err != nil {
		return fmt.Errorf("failed to zero-out bytes: %w", err)
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
