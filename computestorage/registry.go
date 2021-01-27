package computestorage

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/Microsoft/go-winio/pkg/guid"
)

func bcdExec(storePath string, args ...string) error {
	var out bytes.Buffer
	argsArr := []string{"/store", storePath, "/offline"}
	argsArr = append(argsArr, args...)
	cmd := exec.Command("bcdedit.exe", argsArr...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bcd command (%s) failed: %s", cmd, err)
	}
	return nil
}

// A registry configuration required for the uvm.
func setBcdRestartOnFailure(storePath string) error {
	return bcdExec(storePath, "/set", "{default}", "restartonfailure", "yes")
}

// A registry configuration required for the uvm.
func setBcdVmbusBootDevice(storePath string) error {
	vmbusDeviceStr := "vmbus={c63c9bdf-5fa5-4208-b03f-6b458b365592}"
	if err := bcdExec(storePath, "/set", "{default}", "device", vmbusDeviceStr); err != nil {
		return err
	}

	if err := bcdExec(storePath, "/set", "{default}", "osdevice", vmbusDeviceStr); err != nil {
		return err
	}

	if err := bcdExec(storePath, "/set", "{bootmgr}", "alternatebootdevice", vmbusDeviceStr); err != nil {
		return err
	}
	return nil
}

// A registry configuration required for the uvm.
func setBcdOsArcDevice(storePath string, diskID, partitionID guid.GUID) error {
	return bcdExec(storePath, "/set", "{default}", "osarcdevice", fmt.Sprintf("gpt_partition={%s};{%s}", diskID, partitionID))
}

// updateBcdStoreForBoot Updates the bcd store at path `storePath` to boot with the disk
// with given ID and given partitionID.
func updateBcdStoreForBoot(storePath string, diskID, partitionID guid.GUID) error {
	if err := setBcdRestartOnFailure(storePath); err != nil {
		return err
	}

	if err := setBcdVmbusBootDevice(storePath); err != nil {
		return err
	}

	if err := setBcdOsArcDevice(storePath, diskID, partitionID); err != nil {
		return err
	}
	return nil
}
