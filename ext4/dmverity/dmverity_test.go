package dmverity

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

func tempFileWithContentLength(t *testing.T, length int) *os.File {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("failed to create temp file")
	}
	defer tmpFile.Close()

	content := make([]byte, length)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("failed to write random bytes to buffer")
	}
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write random bytes to temp file")
	}
	return tmpFile
}

func writeDMVeritySuperBlock(filename string) (*os.File, error) {
	out, err := os.OpenFile(filename, os.O_RDWR, 0777)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	fsSize, err := out.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	sb := NewDMVeritySuperblock(uint64(fsSize))
	if err := binary.Write(out, binary.LittleEndian, sb); err != nil {
		return nil, err
	}
	padding := bytes.Repeat([]byte{0}, blockSize-(sbSize%blockSize))
	if _, err = out.Write(padding); err != nil {
		return nil, err
	}
	return out, nil
}

func TestInvalidReadEOF(t *testing.T) {
	tmpFile := tempFileWithContentLength(t, blockSize)
	_, err := ReadDMVerityInfo(tmpFile.Name(), blockSize)
	if err == nil {
		t.Fatalf("no error returned")
	}
	if errors.Cause(err) != io.EOF {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestInvalidReadNotEnoughBytes(t *testing.T) {
	tmpFile := tempFileWithContentLength(t, blockSize+blockSize/2)
	_, err := ReadDMVerityInfo(tmpFile.Name(), blockSize)
	if err == nil {
		t.Fatalf("no error returned")
	}
	if errors.Cause(err) != ErrSuperBlockReadFailure || !strings.Contains(err.Error(), "unexpected bytes read") {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestNotVeritySuperBlock(t *testing.T) {
	tmpFile := tempFileWithContentLength(t, 2*blockSize)
	_, err := ReadDMVerityInfo(tmpFile.Name(), blockSize)
	if err == nil {
		t.Fatalf("no error returned")
	}
	if err != ErrNotVeritySuperBlock {
		t.Fatalf("expected %q, got %q", ErrNotVeritySuperBlock, err)
	}
}

func TestNoMerkleTree(t *testing.T) {
	tmpFile := tempFileWithContentLength(t, blockSize)
	targetFile, err := writeDMVeritySuperBlock(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to write dm-verity super-block: %s", err)
	}
	_, err = ReadDMVerityInfo(targetFile.Name(), blockSize)
	if err == nil {
		t.Fatalf("no error returned")
	}
	if errors.Cause(err) != io.EOF || !strings.Contains(err.Error(), "failed to read dm-verity root hash") {
		t.Fatalf("expected %q, got %q", io.EOF, err)
	}
}
