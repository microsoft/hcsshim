//go:build linux
// +build linux

package crypt

import (
	"context"
	"testing"

	"github.com/pkg/errors"
)

const tempDir = "/tmp/dir/"

func osMkdirTempTest(dir string, pattern string) (string, error) {
	return tempDir, nil
}

func clearCryptTestDependencies() {
	_cryptsetupClose = nil
	_cryptsetupFormat = nil
	_cryptsetupOpen = nil
	_generateKeyFile = nil
	_osMkdirTemp = osMkdirTempTest
	_osRemoveAll = nil
	_zeroFirstBlock = nil
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
	_cryptsetupFormat = func(_ context.Context, source string, keyFilePath string) error {
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
	_cryptsetupFormat = func(_ context.Context, source string, keyFilePath string) error {
		return nil
	}

	expectedSource := "/dev/sda"
	dmCryptName := "dm-crypt-target"
	expectedKeyFilePath := tempDir + "keyfile"

	expectedErr := errors.New("expected error message")
	_cryptsetupOpen = func(_ context.Context, source string, deviceName string, keyFilePath string) error {
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

func Test_Encrypt_Success(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when everything goes right.
	_generateKeyFile = func(path string, size int64) error {
		return nil
	}
	_osRemoveAll = func(path string) error {
		return nil
	}
	_cryptsetupFormat = func(_ context.Context, source string, keyFilePath string) error {
		return nil
	}
	_cryptsetupOpen = func(_ context.Context, source string, deviceName string, keyFilePath string) error {
		return nil
	}
	_zeroFirstBlock = func(_ string, _ int) error {
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

	_cryptsetupClose = func(_ context.Context, deviceName string) error {
		if deviceName != dmCryptName {
			t.Fatalf("expected device name: '%s' got: '%s'", dmCryptName, deviceName)
		}
		return expectedErr
	}

	err := CleanupCryptDevice(context.TODO(), dmCryptName)
	if errors.Unwrap(err) != expectedErr {
		t.Fatalf("expected err: '%v' got: '%v'", expectedErr, err)
	}
}

func Test_Cleanup_Dm_Crypt_Success(t *testing.T) {
	clearCryptTestDependencies()

	// Test what happens when cryptsetup succeeds to close an encrypted device.

	_cryptsetupClose = func(_ context.Context, deviceName string) error {
		return nil
	}

	source := "/dev/sda"
	err := CleanupCryptDevice(context.TODO(), source)
	if err != nil {
		t.Fatalf("unexpected err: '%v'", err)
	}
}
