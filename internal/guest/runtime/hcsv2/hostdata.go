//go:build linux
// +build linux

package hcsv2

import (
	"bytes"
	"fmt"

	"github.com/Microsoft/hcsshim/pkg/amdsevsnp"
)

// validateHostData fetches SNP report (if applicable) and validates `hostData` against
// HostData set at UVM launch.
func validateHostData(hostData []byte) error {
	// If the UVM is not SNP, then don't try to fetch an SNP report.
	if !amdsevsnp.IsSNP() {
		return nil
	}
	report, err := amdsevsnp.FetchParsedSNPReport(nil)
	if err != nil {
		return err
	}

	if !bytes.Equal(hostData, report.HostData) {
		return fmt.Errorf(
			"security policy digest %q doesn't match HostData provided at launch %q",
			hostData,
			report.HostData,
		)
	}
	return nil
}
