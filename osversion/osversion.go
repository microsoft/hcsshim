package osversion

import (
	"fmt"
	"strconv"
	"strings"
)

// OSVersion is a wrapper for Windows version information
// https://msdn.microsoft.com/en-us/library/windows/desktop/ms724439(v=vs.85).aspx
type OSVersion struct {
	Version      uint32
	MajorVersion uint8
	MinorVersion uint8
	Build        uint16
}

func newVersion(majorVersion, minorVersion uint8, buildNumber uint16) OSVersion {
	osv := OSVersion{
		MajorVersion: majorVersion,
		MinorVersion: minorVersion,
		Build:        buildNumber,
	}
	// Fill version value so that existing clients don't break
	osv.Version = uint32(buildNumber) << 16
	osv.Version |= uint32(osv.MinorVersion) << 8
	osv.Version |= uint32(osv.MajorVersion)
	return osv
}

// Parse parses a string representation of OSVersion as produced by String
// method.
// The expected format is:
// Major.Minor.Build
//
// The version string may also include a Revision component:
// Major.Minor.Build.Revision
// It will also be parsed but the Revision component will be ignored.
func Parse(str string) (OSVersion, error) {
	p := strings.SplitN(str, ".", 5)
	if len(p) < 3 || len(p) > 4 {
		return OSVersion{}, fmt.Errorf("unexpected OSVersion format %q", str)
	}

	majorVersion, err := strconv.ParseUint(p[0], 10, 8)
	if err != nil {
		return OSVersion{}, fmt.Errorf("major version is not a valid integer %q", p[0])
	}

	minorVersion, err := strconv.ParseUint(p[1], 10, 8)
	if err != nil {
		return OSVersion{}, fmt.Errorf("minor version is not a valid integer %q", p[1])
	}

	buildNumber, err := strconv.ParseUint(p[2], 10, 16)
	if err != nil {
		return OSVersion{}, fmt.Errorf("build number is not a valid integer %q", p[2])
	}

	return newVersion(uint8(majorVersion), uint8(minorVersion), uint16(buildNumber)), nil
}

// String returns the OSVersion formatted as a string. It implements the
// [fmt.Stringer] interface.
func (osv OSVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", osv.MajorVersion, osv.MinorVersion, osv.Build)
}
