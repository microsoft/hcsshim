//go:build windows

// Package devguard reads HKLM\Software\Microsoft\HCS\Dev\Reboot\<Name> DWORDs
// at runtime for the container-reboot-v2 workstream dev matrix.
//
// Behavior: every call opens the registry key, reads the value, closes.
// No caching. On any error, returns false (absent == disabled).
package devguard

import (
	"golang.org/x/sys/windows/registry"
)

const guardRoot = `Software\Microsoft\HCS\Dev\Reboot`

// Guard names mirror the HcsDev::Reboot::* accessors on the HCS C++ side.
const (
	ForceStopForRestart      = "ForceStopForRestart"
	ExposeRebootNotification = "ExposeRebootNotification"
	PassExitStatusJson       = "PassExitStatusJson"
	SkipInternalRebootStart  = "SkipInternalRebootStart"
	EnableShimRebootHandler  = "EnableShimRebootHandler"
)

// IsEnabled returns true iff HKLM\guardRoot\<name> exists as a non-zero DWORD.
// Missing key, missing value, wrong type, or access-denied all return false.
func IsEnabled(name string) bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, guardRoot, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return false
	}
	return v != 0
}
