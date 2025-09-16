//go:build windows
// +build windows

package pspdriver

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
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

// Type used by FetchParsedSNPReport.
// This it converted to the public type `Report`
// by `func (sr *report) report() Report`.
type report struct {
	Version          uint32
	GuestSVN         uint32
	Policy           uint64
	FamilyID         [16]byte
	ImageID          [16]byte
	VMPL             uint32
	SignatureAlgo    uint32
	PlatformVersion  uint64
	PlatformInfo     uint64
	AuthorKeyEn      uint32
	Reserved1        uint32
	ReportData       [SnpPspReportDataSize]byte
	Measurement      [48]byte
	HostData         [SnpPspReportHostDataSize]byte
	IDKeyDigest      [48]byte
	AuthorKeyDigest  [48]byte
	ReportID         [32]byte
	ReportIDMA       [32]byte
	ReportTCB        uint64
	Reserved2        [24]byte
	ChipID           [64]byte
	CommittedSVN     [8]byte
	CommittedVersion [8]byte
	LaunchSVN        [8]byte
	Reserved3        [168]byte
	Signature        [512]byte
}

// Report represents parsed attestation report.
// Fields with string type is hex-encoded values of the corresponding byte arrays.
// Based on Table 23 of 'SEV-ES Guest-Hypervisor Communication Block Standardization'.
//
//	https://www.amd.com/content/dam/amd/en/documents/epyc-technical-docs/specifications/56421.pdf
type Report struct {
	Version          uint32
	GuestSVN         uint32
	Policy           uint64
	FamilyID         string
	ImageID          string
	VMPL             uint32
	SignatureAlgo    uint32
	PlatformVersion  uint64
	PlatformInfo     uint64
	AuthorKeyEn      uint32
	ReportData       string
	Measurement      string
	HostData         []byte
	IDKeyDigest      string
	AuthorKeyDigest  string
	ReportID         string
	ReportIDMA       string
	ReportTCB        uint64
	ChipID           string
	CommittedSVN     string
	CommittedVersion string
	LaunchSVN        string
	Signature        string
}

func (sr *report) report() Report {
	return Report{
		Version:          sr.Version,
		GuestSVN:         sr.GuestSVN,
		Policy:           sr.Policy,
		FamilyID:         hex.EncodeToString(mirrorBytes(sr.FamilyID[:])[:]),
		ImageID:          hex.EncodeToString(mirrorBytes(sr.ImageID[:])[:]),
		VMPL:             sr.VMPL,
		SignatureAlgo:    sr.SignatureAlgo,
		PlatformVersion:  sr.PlatformVersion,
		PlatformInfo:     sr.PlatformInfo,
		AuthorKeyEn:      sr.AuthorKeyEn,
		ReportData:       hex.EncodeToString(sr.ReportData[:]),
		Measurement:      hex.EncodeToString(sr.Measurement[:]),
		HostData:         sr.HostData[:],
		IDKeyDigest:      hex.EncodeToString(sr.IDKeyDigest[:]),
		AuthorKeyDigest:  hex.EncodeToString(sr.AuthorKeyDigest[:]),
		ReportID:         hex.EncodeToString(sr.ReportID[:]),
		ReportIDMA:       hex.EncodeToString(sr.ReportIDMA[:]),
		ReportTCB:        sr.ReportTCB,
		ChipID:           hex.EncodeToString(sr.ChipID[:]),
		CommittedSVN:     hex.EncodeToString(sr.CommittedSVN[:]),
		CommittedVersion: hex.EncodeToString(sr.CommittedVersion[:]),
		LaunchSVN:        hex.EncodeToString(sr.LaunchSVN[:]),
		Signature:        hex.EncodeToString(sr.Signature[:]),
	}
}

// mirrorBytes mirrors the byte ordering so that hex-encoding little endian
// ordered bytes come out in the readable order.
func mirrorBytes(b []byte) []byte {
	for i := 0; i < len(b)/2; i++ {
		mirrorIndex := len(b) - i - 1
		b[i], b[mirrorIndex] = b[mirrorIndex], b[i]
	}
	return b
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
func GetPspDriverError() error {
	return pspDriverError
}

// IsSNPMode() returns true if it's in SNP mode.
func IsSNPMode(ctx context.Context) (bool, error) {

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
func FetchRawSNPReport(ctx context.Context, reportData []byte) ([]byte, error) {
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

// FetchParsedSNPReport parses raw attestation response into proper structs.
func FetchParsedSNPReport(ctx context.Context, reportData []byte) (Report, error) {
	rawBytes, err := FetchRawSNPReport(ctx, reportData)
	if err != nil {
		return Report{}, err
	}

	var r report
	buf := bytes.NewBuffer(rawBytes)
	if err := binary.Read(buf, binary.LittleEndian, &r); err != nil {
		return Report{}, err
	}
	return r.report(), nil
}

// TODO: Based on internal\guest\runtime\hcsv2\hostdata.go and it's duplicated.
// ValidateHostData fetches SNP report (if applicable) and validates `hostData` against
// HostData set at UVM launch.
func ValidateHostData(ctx context.Context, hostData []byte) error {
	// If the UVM is not SNP, then don't try to fetch an SNP report.
	isSnpMode, err := IsSNPMode(ctx)
	if err != nil {
		return err
	}
	if !isSnpMode {
		return nil
	}
	report, err := FetchParsedSNPReport(ctx, nil)
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
