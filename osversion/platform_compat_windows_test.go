package osversion

import (
	"testing"
)

// Test the platform compatibility of the different
// OS Versions considering two ltsc container image
// versions (ltsc2019, ltsc2022)
func Test_PlatformCompat(t *testing.T) {
	for testName, tc := range map[string]struct {
		hostOs    uint16
		ctrOs     uint16
		shouldRun bool
	}{
		"RS5Host_ltsc2019": {
			hostOs:    RS5,
			ctrOs:     RS5,
			shouldRun: true,
		},
		"RS5Host_ltsc2022": {
			hostOs:    RS5,
			ctrOs:     V21H2Server,
			shouldRun: false,
		},
		"WS2022Host_ltsc2019": {
			hostOs:    V21H2Server,
			ctrOs:     RS5,
			shouldRun: false,
		},
		"WS2022Host_ltsc2022": {
			hostOs:    V21H2Server,
			ctrOs:     V21H2Server,
			shouldRun: true,
		},
		"Wind11Host_ltsc2019": {
			hostOs:    V22H2Win11,
			ctrOs:     RS5,
			shouldRun: false,
		},
		"Wind11Host_ltsc2022": {
			hostOs:    V22H2Win11,
			ctrOs:     V21H2Server,
			shouldRun: true,
		},
	} {
		// Check if ltsc2019/ltsc2022 guest images are compatible on
		// the given host OS versions
		//
		hostOSVersion := OSVersion{
			MajorVersion: 10,
			MinorVersion: 0,
			Build:        tc.hostOs,
		}
		ctrOSVersion := OSVersion{
			MajorVersion: 10,
			MinorVersion: 0,
			Build:        tc.ctrOs,
		}
		if CheckHostAndContainerCompat(hostOSVersion, ctrOSVersion) != tc.shouldRun {
			var expectedResultStr string
			if !tc.shouldRun {
				expectedResultStr = " NOT"
			}
			t.Fatalf("Failed %v: host %v should%s be able to run guest %v", testName, tc.hostOs, expectedResultStr, tc.ctrOs)
		}
	}
}
