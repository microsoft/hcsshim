//go:build windows

package winapi

import (
	"golang.org/x/sys/windows"
)

func IsEvelated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
