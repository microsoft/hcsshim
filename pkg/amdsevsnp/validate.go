package amdsevsnp

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
)

// validateHostData fetches SNP report (if applicable) and validates `hostData` against
// HostData set at UVM launch.
func ValidateHostData(hostData []byte) error {

	if err := CheckDriverError(); err != nil {
		// For this case gcs-sidecar will keep initial deny policy.
		return errors.Wrapf(err, "an error occurred while using PSP driver")
	}
	// If the UVM is not SNP, then don't try to fetch an SNP report.
	isSNP, err := IsSNP()
	if err != nil {
		return err
	}
	if !isSNP {
		return nil
	}
	report, err := FetchParsedSNPReport(nil)
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
