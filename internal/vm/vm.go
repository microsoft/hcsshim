//go:build windows

package vm

// GuestOS signifies the guest operating system that a Utility VM will be running.
type GuestOS string

const (
	Windows GuestOS = "windows"
	Linux   GuestOS = "linux"
)
