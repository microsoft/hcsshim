//go:build linux
// +build linux

package crypt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	_cryptsetupClose  = cryptsetupClose
	_cryptsetupFormat = cryptsetupFormat
	_cryptsetupOpen   = cryptsetupOpen
	_generateKeyFile  = generateKeyFile
	_osMkdirTemp      = os.MkdirTemp
	_osRemoveAll      = os.RemoveAll
	_zeroFirstBlock   = zeroFirstBlock
)

// cryptsetupCommand runs cryptsetup with the provided arguments
func cryptsetupCommand(ctx context.Context, args []string) error {
	// Occasionally the /dev/sd* arrive with a delay and one of the errors below
	// could be returned as a result. Keep retrying cryptsetup command until
	// successful or a context deadline is reached.
	retryErrors := []string{
		fmt.Sprint(unix.ENOENT),
		fmt.Sprint(unix.ENXIO),
		fmt.Sprint(unix.ENODEV),
	}
retry:
	for {
		// --debug and -v are used to increase the information printed by
		// cryptsetup. By default, it doesn't print much information, which makes it
		// hard to debug it when there are problems.
		cmd := exec.Command("cryptsetup", append([]string{"--debug", "-v"}, args...)...)
		output, err := cmd.CombinedOutput()
		strOutput := string(output)
		if err != nil {
			log.G(ctx).WithError(err).WithFields(logrus.Fields{
				"args":   args,
				"output": string(output),
			}).Warning("cryptsetup failed")
			for _, e := range retryErrors {
				if strings.Contains(strOutput, e) {
					select {
					case <-ctx.Done():
						log.G(ctx).WithError(err).Warning("cryptsetup failed, context timeout")
						return fmt.Errorf("cryptsetup failed: %w", err)
					default:
						time.Sleep(100 * time.Millisecond)
						continue retry
					}
				}
			}
			return fmt.Errorf("cryptsetup failed: %w", err)
		}
		return nil
	}
}

// cryptsetupFormat runs "cryptsetup luksFormat" with the right arguments to use
// dm-crypt and dm-integrity.
func cryptsetupFormat(ctx context.Context, source string, keyFilePath string) error {
	formatArgs := []string{
		// Mount source using LUKS2
		"luksFormat", source, "--type", "luks2",
		// Provide keyfile and prevent showing the confirmation prompt
		"--key-file", keyFilePath, "--batch-mode",
		// dm-crypt and dm-integrity algorithms. The dm-crypt algorithm is the
		// default one used for LUKS. The dm-integrity is the one the
		// documentation mentions as one of the combinations they use for
		// testing:
		// https://gitlab.com/cryptsetup/cryptsetup/-/blob/a0277d3ff6ab7d5c9e0534f25b4b40719e999c8e/docs/v2.0.0-ReleaseNotes#L259-261
		"--cipher", "aes-xts-plain64", "--integrity", "hmac-sha256",
		// See EncryptDevice() for the reason of using --integrity-no-wipe
		"--integrity-no-wipe",
		// Use 4KB sectors, the documentation mentions it can improve
		// performance than smaller sizes.
		"--sector-size", "4096",
		// Force PBKDF2 and a specific number of iterations to skip the
		// benchmarking step of luksFormat. Using a KDF is required by
		// cryptsetup. The reason why it is mandatory to use a KDF is that
		// cryptsetup expects the user to input a passphrase and cryptsetup is
		// supposed to derive a strong key from it. In our case, we already pass
		// a strong key to cryptsetup, so we don't need a strong KDF. Ideally,
		// it would be bypassed completely, but this isn't possible.
		"--pbkdf", "pbkdf2", "--pbkdf-force-iterations", "1000"}

	return cryptsetupCommand(ctx, formatArgs)
}

// cryptsetupOpen runs "cryptsetup luksOpen" with the right arguments.
func cryptsetupOpen(ctx context.Context, source string, deviceName string, keyFilePath string) error {
	openArgs := []string{
		// Open device with the key passed to luksFormat
		"luksOpen", source, deviceName, "--key-file", keyFilePath,
		// Don't use a journal to increase performance
		"--integrity-no-journal", "--persistent"}

	return cryptsetupCommand(ctx, openArgs)
}

// cryptsetupClose runs "cryptsetup luksClose" with the right arguments.
func cryptsetupClose(ctx context.Context, deviceName string) error {
	closeArgs := []string{"luksClose", deviceName}

	return cryptsetupCommand(ctx, closeArgs)
}

// EncryptDevice creates a dm-crypt target for a container scratch vhd.
//
// In order to mount a block device as an encrypted device:
//
//  1. Generate a random key. It doesn't matter which key it is, the aim is to
//     protect the contents of the scratch disk from the host OS. It can be
//     deleted after mounting the encrypted device.
//
//  2. The original block device has to be formatted with cryptsetup with the
//     generated key. This results in that block device becoming an encrypted
//     block device that can't be mounted directly.
//
//  3. Open the block device with cryptsetup. It is needed to assign it a device
//     name. We are using names that follow `cryptDeviceTemplate`, where "%s" is
//     a unique name generated from the path of the original block device. In
//     this case, it's just the path of the block device with all
//     non-alphanumeric characters replaced by a '-'.
//
//     The kernel exposes the unencrypted block device at the path
//     /dev/mapper/`cryptDeviceTemplate`. This can be mounted directly, but it
//     doesn't have any format yet.
//
//  4. Prepare the unecrypted block device to be later formatted as xfs
//  4.1. Zero the first block. It appears that mkfs.xfs reads this before formatting.

func EncryptDevice(ctx context.Context, source string, dmCryptName string) (path string, err error) {
	// Create temporary directory to store the keyfile and xfs image
	tempDir, err := _osMkdirTemp("", "dm-crypt")
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temporary folder: %s", source)
	}

	defer func() {
		// Delete it on exit, it won't be needed afterwards
		if err := _osRemoveAll(tempDir); err != nil {
			log.G(ctx).WithError(err).Debugf("failed to delete temporary folder: %s", tempDir)
		}
	}()

	// 1. Generate keyfile
	keyFilePath := filepath.Join(tempDir, "keyfile")
	if err = _generateKeyFile(keyFilePath, 1024); err != nil {
		return "", fmt.Errorf("failed to generate keyfile %q: %w", keyFilePath, err)
	}

	// 2. Format device
	if err = _cryptsetupFormat(ctx, source, keyFilePath); err != nil {
		return "", fmt.Errorf("luksFormat failed: %s: %w", source, err)
	}

	// 3. Open device
	if err := _cryptsetupOpen(ctx, source, dmCryptName, keyFilePath); err != nil {
		return "", fmt.Errorf("luksOpen failed: %s: %w", source, err)
	}

	defer func() {
		if err != nil {
			if inErr := CleanupCryptDevice(ctx, source); inErr != nil {
				log.G(ctx).WithError(inErr).Debug("failed to cleanup crypt device")
			}
		}
	}()

	deviceNamePath := "/dev/mapper/" + dmCryptName
	// 4.1. Zero the first block.
	// In the xfs mkfs case it appears to attempt to read the first block of the device.
	// This results in an integrity error. This function zeros out the start of the device,
	// so we are sure that when it is read it has already been hashed so matches.
	if err := _zeroFirstBlock(deviceNamePath, 4096); err != nil {
		return "", fmt.Errorf("failed to zero first block: %w", err)
	}

	return deviceNamePath, nil
}

// CleanupCryptDevice removes the dm-crypt device created by EncryptDevice
func CleanupCryptDevice(ctx context.Context, dmCryptName string) error {
	// Close dm-crypt device
	if err := _cryptsetupClose(ctx, dmCryptName); err != nil {
		return fmt.Errorf("luksClose failed: %s: %w", dmCryptName, err)
	}
	return nil
}
