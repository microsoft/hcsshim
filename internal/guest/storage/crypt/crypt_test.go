//go:build linux
// +build linux

package crypt

import (
	"context"
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/internal/memory"
	"github.com/pkg/errors"
)

const tempDir = "/tmp/dir/"

func osMkdirTempTest(dir string, pattern string) (string, error) {
	return tempDir, nil
}

func clearCryptTestDependencies() {
	_copyEmptySparseFilesystem = nil
	_createSparseEmptyFile = nil
	_cryptsetupClose = nil
	_cryptsetupFormat = nil
	_cryptsetupOpen = nil
	_generateKeyFile = nil
	_getBlockDeviceSize = nil
	_mkfsExt4Command = nil
	_osMkdirTemp = osMkdirTempTest
	_osRemoveAll = nil
}

func Test_Encrypt_Generate_Key_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when key generation fails for any reason. Verify that
	// the generated keyfile path has a number that matches the index value.

	source := "/dev/sda"
	keyfilePath := tempDir + "keyfile"
	expectedErr := errors.New("expected error message")

	_osRemoveAll = func(path string) error {
		return nil
	}
	_generateKeyFile = func(path string, size int64) error {
		if keyfilePath != path {
			t.Errorf("expected path: %v, got: %v", keyfilePath, path)
		}
		return expectedErr
	}

	_, err := EncryptDevice(context.Background(), source, "dm-crypt-target")
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Encrypt_Cryptsetup_Format_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when cryptsetup fails to format the device. Verify that
	// the arguments passed to cryptsetup are the right ones.

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}

	expectedSource := "/dev/sda"
	expectedKeyFilePath := tempDir + "keyfile"

	expectedErr := errors.New("expected error message")
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		if source != expectedSource {
			t.Fatalf("expected source: '%s' got: '%s'", expectedSource, source)
		}
		if keyFilePath != expectedKeyFilePath {
			t.Fatalf("expected keyfile path: '%s' got: '%s'", expectedKeyFilePath, keyFilePath)
		}
		return expectedErr
	}

	_, err := EncryptDevice(context.Background(), expectedSource, "dm-crypt-target")
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v", expectedErr, err)
	}
}

func Test_Encrypt_Cryptsetup_Open_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when cryptsetup fails to open the device. Verify that
	// the arguments passed to cryptsetup are the right ones.

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		return nil
	}

	expectedSource := "/dev/sda"
	dmCryptName := "dm-crypt-target"
	expectedKeyFilePath := tempDir + "keyfile"

	expectedErr := errors.New("expected error message")
	_cryptsetupOpen = func(source string, deviceName string, keyFilePath string) error {
		if source != expectedSource {
			t.Fatalf("expected source: '%s' got: '%s'", expectedSource, source)
		}
		if deviceName != dmCryptName {
			t.Fatalf("expected device name: '%s' got: '%s'", dmCryptName, deviceName)
		}
		if keyFilePath != expectedKeyFilePath {
			t.Fatalf("expected keyfile path: '%s' got: '%s'", expectedKeyFilePath, keyFilePath)
		}
		return expectedErr
	}

	_, err := EncryptDevice(context.Background(), expectedSource, dmCryptName)
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Encrypt_Get_Device_Size_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when cryptsetup fails to get the size of the
	// unencrypted block device.

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		return nil
	}
	_cryptsetupOpen = func(source string, deviceName string, keyFilePath string) error {
		return nil
	}
	_cryptsetupClose = func(deviceName string) error {
		return nil
	}

	source := "/dev/sda"
	dmCryptName := "dm-crypt-target"
	deviceNamePath := "/dev/mapper/" + dmCryptName

	expectedErr := errors.New("expected error message")
	_getBlockDeviceSize = func(ctx context.Context, path string) (int64, error) {
		return 0, expectedErr
	}

	_, err := EncryptDevice(context.Background(), source, dmCryptName)
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}

	// Check that it fails when the size of the block device is zero

	expectedErr = fmt.Errorf("invalid size obtained for: %s", deviceNamePath)
	_getBlockDeviceSize = func(ctx context.Context, path string) (int64, error) {
		return 0, nil
	}

	_, err = EncryptDevice(context.Background(), source, dmCryptName)
	if err.Error() != expectedErr.Error() {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Encrypt_Create_Sparse_File_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when it isn't possible to create a sparse file, and
	// make sure that _createSparseEmptyFile receives the right arguments.

	blockDeviceSize := int64(memory.GiB)

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		return nil
	}
	_cryptsetupOpen = func(source string, deviceName string, keyFilePath string) error {
		return nil
	}
	_cryptsetupClose = func(deviceName string) error {
		return nil
	}
	_getBlockDeviceSize = func(ctx context.Context, path string) (int64, error) {
		// Return a non-zero size
		return blockDeviceSize, nil
	}

	source := "/dev/sda"
	tempExt4File := tempDir + "ext4.img"

	expectedErr := errors.New("expected error message")
	_createSparseEmptyFile = func(ctx context.Context, path string, size int64) error {
		// Check that the path and the size are the expected ones
		if path != tempExt4File {
			t.Fatalf("expected path: '%v' got: '%v'", tempExt4File, path)
		}
		if size != blockDeviceSize {
			t.Fatalf("expected size: '%v' got: '%v'", blockDeviceSize, size)
		}

		return expectedErr
	}

	_, err := EncryptDevice(context.Background(), source, "dm-crypt-name")
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Encrypt_Mkfs_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when mkfs fails to format the unencrypted device.
	// Verify that the arguments passed to it are the right ones.

	blockDeviceSize := int64(memory.GiB)

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		return nil
	}
	_cryptsetupOpen = func(source string, deviceName string, keyFilePath string) error {
		return nil
	}
	_cryptsetupClose = func(deviceName string) error {
		return nil
	}
	_getBlockDeviceSize = func(ctx context.Context, path string) (int64, error) {
		// Return a non-zero size
		return blockDeviceSize, nil
	}
	_createSparseEmptyFile = func(ctx context.Context, path string, size int64) error {
		return nil
	}

	source := "/dev/sda"
	tempExt4File := tempDir + "ext4.img"

	expectedErr := errors.New("expected error message")
	_mkfsExt4Command = func(args []string) error {
		if args[0] != tempExt4File {
			t.Fatalf("expected args: '%v' got: '%v'", tempExt4File, args[0])
		}
		return expectedErr
	}

	_, err := EncryptDevice(context.Background(), source, "dm-crypt-name")
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Encrypt_Sparse_Copy_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when the sparse copy fails. Verify that the arguments
	// passed to it are the right ones.

	blockDeviceSize := int64(memory.GiB)

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		return nil
	}
	_cryptsetupOpen = func(source string, deviceName string, keyFilePath string) error {
		return nil
	}
	_cryptsetupClose = func(deviceName string) error {
		return nil
	}
	_getBlockDeviceSize = func(ctx context.Context, path string) (int64, error) {
		// Return a non-zero size
		return blockDeviceSize, nil
	}
	_createSparseEmptyFile = func(ctx context.Context, path string, size int64) error {
		return nil
	}
	_mkfsExt4Command = func(args []string) error {
		return nil
	}

	source := "/dev/sda"
	tempExt4File := tempDir + "ext4.img"
	dmCryptName := "dm-crypt-target"
	deviceNamePath := "/dev/mapper/" + dmCryptName

	expectedErr := errors.New("expected error message")
	_copyEmptySparseFilesystem = func(source string, destination string) error {
		if source != tempExt4File {
			t.Fatalf("expected source: '%v' got: '%v'", tempExt4File, source)
		}
		if destination != deviceNamePath {
			t.Fatalf("expected destination: '%v' got: '%v'", deviceNamePath, destination)
		}
		return expectedErr
	}

	_, err := EncryptDevice(context.Background(), source, dmCryptName)
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err:'%v' got: '%v'", expectedErr, err)
	}
}

func Test_Encrypt_Success(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when everything goes right.

	blockDeviceSize := int64(memory.GiB)

	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(source string, keyFilePath string) error {
		return nil
	}
	_cryptsetupOpen = func(source string, deviceName string, keyFilePath string) error {
		return nil
	}
	_getBlockDeviceSize = func(ctx context.Context, path string) (int64, error) {
		// Return a non-zero size
		return blockDeviceSize, nil
	}
	_createSparseEmptyFile = func(ctx context.Context, path string, size int64) error {
		return nil
	}
	_mkfsExt4Command = func(args []string) error {
		return nil
	}
	_copyEmptySparseFilesystem = func(source string, destination string) error {
		return nil
	}

	source := "/dev/sda"
	dmCryptName := "dm-crypt-name"
	deviceNamePath := "/dev/mapper/" + dmCryptName

	encryptedSource, err := EncryptDevice(context.Background(), source, dmCryptName)
	if err != nil {
		t.Fatalf("unexpected err: '%v'", err)
	}
	if deviceNamePath != encryptedSource {
		t.Fatalf("expected path: '%v' got: '%v'", deviceNamePath, encryptedSource)
	}
}

func Test_Cleanup_Dm_Crypt_Error(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when cryptsetup fails to remove an encrypted device.
	// Verify that the arguments passed to cryptsetup are the right ones.

	dmCryptName := "dm-crypt-target"
	expectedErr := errors.New("expected error message")

	_cryptsetupClose = func(deviceName string) error {
		if deviceName != dmCryptName {
			t.Fatalf("expected device name: '%s' got: '%s'", dmCryptName, deviceName)
		}
		return expectedErr
	}

	err := CleanupCryptDevice(dmCryptName)
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Cleanup_Dm_Crypt_Success(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when cryptsetup succeeds to close an encrypted device.

	_cryptsetupClose = func(deviceName string) error {
		return nil
	}

	source := "/dev/sda"
	err := CleanupCryptDevice(source)
	if err != nil {
		t.Fatalf("unexpected err: '%v'", err)
	}
}
