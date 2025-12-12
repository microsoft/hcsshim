//go:build windows
// +build windows

package amdsevsnp

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/winapi"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName = "AmdSnpPsp"
)

const (
	SnpPspAPIStatusSuccess            = 0x00000000
	SnpPspAPIStatusUnsuccessful       = 0x00000001
	SnpPspAPIStatusDriverUnsuccessful = 0x00000002
	SnpPspAPIStatusPspUnsuccessful    = 0x00000003
	SnpPspAPIStatusInvalidParameter   = 0x00000004
	SnpPspAPIStatusDeviceNotAvailable = 0x00000005
)

// TODO: Fix duplication with pkg/amdsevsnp and merge this into it.

const (
	SnpPspReportDataSize        = 64
	SnpPspReportHostDataSize    = 32
	SnpPspAttestationReportSize = 0x4A0
)

type SNPPSPGuestRequestResult struct {
	DriverStatus uint32
	PspStatus    uint64
}

var (
	pspDriverStarted = false
	// The error needs to be stored to be retrieved later.
	// When driver or its dll fails, gcs-sidecar doesn't
	// set security policy and keep the initial deny policy.
	pspDriverError error = nil
)

func StartPSPDriver(ctx context.Context) error {
	// Connect to the Service Control Manager
	m, err := mgr.Connect()
	if err != nil {
		return errors.Wrap(err, "Failed to connect to service manager")
	}
	defer func() {
		if derr := m.Disconnect(); derr != nil {
			// Log the error on disconnect but do not override the returned error.
			log.G(ctx).Warnf("Failed to disconnect from service manager: %v", derr)
		}
	}()

	// Open the service
	s, err := m.OpenService(serviceName)
	if err != nil {
		return errors.Wrapf(err, "Could not access service %q", serviceName)
	}
	defer s.Close()

	// Start the service
	err = s.Start()
	if err != nil {
		return errors.Wrapf(err, "Could not start service %q", serviceName)
	}

	// From the documentation, there is no guarantee that the service will be
	// in `Running` state immediately after starting it.
	// Wait until the service is in the `Running` state.
	timeout := time.After(3 * time.Second)
	tick := time.Tick(100 * time.Millisecond)
	for {
		select {
		case <-timeout:
			pspDriverError = errors.New("timed out waiting for PSP driver to start")
			return pspDriverError
		case <-tick:
			status, err := s.Query()
			if err != nil {
				pspDriverError = errors.Wrap(err, "could not query PSP driver status")
				return pspDriverError
			}
			if status.State == svc.Running {
				log.G(ctx).Tracef("Service %q started successfully", serviceName)

				pspDriverStarted = true
				return nil
			}
		}
	}
}

func IsPspDriverStarted() bool {
	return pspDriverStarted
}

// Return an error from the PSP driver dll
// when it fails to use the dll at all.
// Otherwise it returns nil.
func CheckDriverError() error {
	return pspDriverError
}

// IsSNP() returns true if it's in SNP mode.
func IsSNP() (bool, error) {

	if pspDriverError != nil {
		return false, pspDriverError
	}

	if !pspDriverStarted {
		return false, errors.New("PSP driver is not started")
	}

	// snpMode is defined as BOOLEAN (= byte)
	var snpMode uint8
	ret, err := winapi.SnpPspIsSnpMode(&snpMode)

	if ret != SnpPspAPIStatusSuccess || err != nil {
		errMessage := ""
		if err != nil {
			// err is not nil either when `winapi` didn't find the API or when ret is not success.
			// In case of the former, ret is meaningless because ret is returned by the dll.
			// In case of the latter, we don't need to print err.
			// We can't tell which case it is here, we print all the information we have.
			// We could avoid this by loading the dll in this package, but we use `winapi` for consistency with existing code.
			errMessage = fmt.Sprintf(", err: %v", err)
		}
		pspDriverError = errors.Errorf("failed to determine if it's in SNP VM. SNPPSP_API_STATUS: 0x%x%s", ret, errMessage)
		return false, pspDriverError
	}

	return snpMode == 1, nil
}

// FetchRawSNPReport returns attestation report bytes.
func FetchRawSNPReport(reportData []byte) ([]byte, error) {
	if pspDriverError != nil {
		return nil, pspDriverError
	}

	if !pspDriverStarted {
		return nil, errors.New("PSP driver is not started")
	}

	var reportDataBuf [SnpPspReportDataSize]uint8

	if reportData != nil {
		if len(reportData) > SnpPspReportDataSize {
			return nil, fmt.Errorf("reportData too large: %s", reportData)
		}
		copy(reportDataBuf[:], reportData)
	}

	var report [SnpPspAttestationReportSize]uint8
	var guestRequestResult winapi.SNPPSPGuestRequestResult

	// Fetch attestation report using generated winapi wrapper
	ret, err := winapi.SnpPspFetchAttestationReport(&reportDataBuf[0], &guestRequestResult, &report[0])
	if ret != SnpPspAPIStatusSuccess || err != nil {
		errMessage := ""
		if err != nil {
			// err is not nil either when `winapi` didn't find the API or when ret is not success.
			// In case of the former, ret and guestRequestResult are meaningless because they are returned by the dll.
			// In case of the latter, we don't need to print err.
			// We can't tell which case it is here, we print all the information we have.
			// We could avoid this by loading the dll in this package, but we use `winapi` for consistency with existing code.
			errMessage = fmt.Sprintf(", err: %v", err)
		}
		pspDriverError = errors.Errorf("failed to fetch attestation report. res: 0x%x, DriverStatus: 0x%x, PspStatus: 0x%x%s",
			ret, guestRequestResult.DriverStatus, guestRequestResult.PspStatus, errMessage)
		return nil, pspDriverError
	}

	return report[:], nil
}

// ValidateHostData fetches SNP report (if applicable) and validates `hostData` against
// HostData set at UVM launch.
func ValidateHostDataPSP(hostData []byte) error {
	// If the UVM is not SNP, then don't try to fetch an SNP report.
	isSnpMode, err := IsSNP()
	if err != nil {
		return err
	}
	if !isSnpMode {
		return nil
	}
	report, err := FetchParsedSNPReport(nil)
	if err != nil {
		return err
	}

	if !bytes.Equal(hostData, report.HostData[:]) {
		return fmt.Errorf(
			"security policy digest %q doesn't match HostData provided at launch %q",
			hostData,
			report.HostData[:],
		)
	}

	return nil
}
