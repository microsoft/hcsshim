//go:build linux
// +build linux

// Package amdsevsnp contains minimal functionality required to fetch
// attestation reports inside an enlightened guest.
package amdsevsnp

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"unsafe"

	"github.com/Microsoft/hcsshim/internal/guest/linux"
)

const (
	msgTypeInvalid = iota
	msgCPUIDRequest
	msgCPUIDResponse
	msgKeyRequest
	msgKeyResponse
	msgReportRequest
	msgReportResponse
	msgExportRequest
	msgExportResponse
	msgImportRequest
	msgImportResponse
	msgAbsorbRequest
	msgAbsorbResponse
	msgVMRKRequest
	msgVMRKResponse
	msgTypeMax
)

type guestRequest struct {
	RequestMsgType  byte
	ResponseMsgType byte
	MsgVersion      byte
	RequestLength   uint16
	RequestUAddr    unsafe.Pointer
	ResponseLength  uint16
	ResponseUAddr   unsafe.Pointer
	Error           uint32
}

// AMD SEV ioctl definitions.
const (
	// SEV-SNP IOCTL type.
	guestType = 'S'
	// SEV-SNP IOCTL size, same as unsafe.Sizeof(SevSNPGuestRequest{}).
	guestSize = 40
	ioctlBase = linux.IocWRBase | guestType<<linux.IocTypeShift | guestSize<<linux.IocSizeShift

	// SEV-SNP requests
	reportCode = 0x1
)

// reportRequest used to issue SEV-SNP request
// https://www.amd.com/system/files/TechDocs/56860.pdf
// MSG_REPORT_REQ Table 20.
type reportRequest struct {
	ReportData [64]byte
	VMPL       uint32
	_          [28]byte
}

// report is an internal representation of SEV-SNP report
// https://www.amd.com/system/files/TechDocs/56860.pdf
// ATTESTATION_REPORT Table 21.
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
	ReportData       [64]byte
	Measurement      [48]byte
	HostData         [32]byte
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

// reportResponse is the attestation response struct
// https://www.amd.com/system/files/TechDocs/56860.pdf
// MSG_REPORT_RSP Table 23.
// NOTE: reportResponse.Report is a byte slice, to have the original
// response in bytes. The conversion to internal struct happens inside
// convertRawReport.
//
// NOTE: the additional 64 bytes are reserved, without them, the ioctl fails.
type reportResponse struct {
	Status     uint32
	ReportSize uint32
	reserved1  [24]byte
	Report     [1184]byte
	reserved2  [64]byte // padding to the size of SEV_SNP_REPORT_RSP_BUF_SZ (i.e., 1280 bytes)
}

// FetchRawSNPReport returns attestation report bytes.
func FetchRawSNPReport(reportData []byte) ([]byte, error) {
	f, err := os.OpenFile("/dev/sev", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var (
		msgReportIn  reportRequest
		msgReportOut reportResponse
	)

	if reportData != nil {
		if len(reportData) > len(msgReportIn.ReportData) {
			return nil, fmt.Errorf("reportData too large: %s", reportData)
		}
		copy(msgReportIn.ReportData[:], reportData)
	}

	payload := &guestRequest{
		RequestMsgType:  msgReportRequest,
		ResponseMsgType: msgReportResponse,
		MsgVersion:      1,
		RequestLength:   uint16(unsafe.Sizeof(msgReportIn)),
		RequestUAddr:    unsafe.Pointer(&msgReportIn),
		ResponseLength:  uint16(unsafe.Sizeof(msgReportOut)),
		ResponseUAddr:   unsafe.Pointer(&msgReportOut),
		Error:           0,
	}

	if err := linux.Ioctl(f, reportCode|ioctlBase, unsafe.Pointer(payload)); err != nil {
		return nil, err
	}
	return msgReportOut.Report[:], nil
}

// Report represents parsed attestation report.
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

// mirrorBytes mirrors the byte ordering so that hex-encoding little endian
// ordered bytes come out in the readable order.
func mirrorBytes(b []byte) []byte {
	for i := 0; i < len(b)/2; i++ {
		mirrorIndex := len(b) - i - 1
		b[i], b[mirrorIndex] = b[mirrorIndex], b[i]
	}
	return b
}

// FetchParsedSNPReport parses raw attestation response into proper structs.
func FetchParsedSNPReport(reportData []byte) (Report, error) {
	rawBytes, err := FetchRawSNPReport(reportData)
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
