//go:build linux
// +build linux

package hcsv2

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/pkg/amdsevsnp"
)

// validateHostData fetches SNP report (if applicable) and validates `hostData` against
// HostData set at UVM launch.
func validateHostData(hostData []byte) error {
	report, err := amdsevsnp.FetchParsedSNPReport(nil)
	if err != nil {
		// For non-SNP hardware /dev/sev will not exist
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if bytes.Compare(hostData, report.HostData) != 0 {
		return fmt.Errorf(
			"security policy digest %q doesn't match HostData provided at launch %q",
			hostData,
			report.HostData,
		)
	}
	return nil
}
