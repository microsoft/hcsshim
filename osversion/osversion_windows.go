package osversion

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	osv  OSVersion
	once sync.Once
)

// Get gets the operating system version on Windows.
// The calling application must be manifested to get the correct version information.
func Get() OSVersion {
	once.Do(func() {
		v := *windows.RtlGetVersion()
		osv = newVersion(uint8(v.MajorVersion), uint8(v.MinorVersion), uint16(v.BuildNumber))
	})
	return osv
}

// Build gets the build-number on Windows
// The calling application must be manifested to get the correct version information.
func Build() uint16 {
	return Get().Build
}

// ToString returns the OSVersion formatted as a string.
//
// Deprecated: use [OSVersion.String].
func (osv OSVersion) ToString() string {
	return osv.String()
}

// Running `cmd /c ver` shows something like "10.0.20348.1000". The last component ("1000") is the revision
// number
func BuildRevision() (uint32, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return 0, fmt.Errorf("open `CurrentVersion` registry key: %w", err)
	}
	defer k.Close()
	s, _, err := k.GetIntegerValue("UBR")
	if err != nil {
		return 0, fmt.Errorf("read `UBR` from registry: %w", err)
	}
	return uint32(s), nil
}
