//go:build linux
// +build linux

package amdsevsnp

import (
	"errors"
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

// AMD SEV ioctl definitions for kernel 5.x.
const (
	snpGetReportIoctlCode5 = 3223868161
)

// AMD SEV ioctl definitions for kernel 6.x.
const (
	snpGetReportIoctlCode6 = 3223343872
)

// reportRequest used to issue SEV-SNP request
// https://www.amd.com/system/files/TechDocs/56860.pdf
// MSG_REPORT_REQ Table 20.
type reportRequest struct {
	ReportData [64]byte
	VMPL       uint32
	_          [28]byte
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

// Size of `snp_report_resp` in include/uapi/linux/sev-guest.h.
// It's used only for Linux 6.x.
// It will have the conteints of reportResponse in the first unsafe.Sizeof(reportResponse{}) bytes.
const reportResponseContainerLength6 = 4000

type guestRequest5 struct {
	RequestMsgType  byte
	ResponseMsgType byte
	MsgVersion      byte
	RequestLength   uint16
	RequestUAddr    unsafe.Pointer
	ResponseLength  uint16
	ResponseUAddr   unsafe.Pointer
	Error           uint32
}

type guestRequest6 struct {
	MsgVersion   byte
	RequestData  unsafe.Pointer
	ResponseData unsafe.Pointer
	Error        uint64
}

const snpDevicePath5 = "/dev/sev"
const snpDevicePath6 = "/dev/sev-guest"

func IsSNP() (bool, error) {
	isVM5, err := isSNPVM5()
	if err != nil {
		return false, err
	}
	if isVM5 {
		return true, nil
	}
	isVM6, err := isSNPVM6()
	if err != nil {
		return false, err
	}
	return isVM6, nil
}

// Check if the code is being run in SNP VM for Linux kernel version 5.x.
func isSNPVM5() (bool, error) {
	_, err := os.Stat(snpDevicePath5)
	return !errors.Is(err, os.ErrNotExist), nil
}

// Check if the code is being run in SNP VM for Linux kernel version 6.x.
func isSNPVM6() (bool, error) {
	_, err := os.Stat(snpDevicePath6)
	return !errors.Is(err, os.ErrNotExist), nil
}

// FetchRawSNPReport returns attestation report bytes.
func FetchRawSNPReport(reportData []byte) ([]byte, error) {
	isVM5, _ := isSNPVM5()
	if isVM5 {
		return fetchRawSNPReport5(reportData)
	}
	isVM6, _ := isSNPVM6()
	if isVM6 {
		return fetchRawSNPReport6(reportData)
	}
	return nil, fmt.Errorf("SEV device is not found")
}

func fetchRawSNPReport5(reportData []byte) ([]byte, error) {
	f, err := os.OpenFile(snpDevicePath5, os.O_RDWR, 0)
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

	payload := &guestRequest5{
		RequestMsgType:  msgReportRequest,
		ResponseMsgType: msgReportResponse,
		MsgVersion:      1,
		RequestLength:   uint16(unsafe.Sizeof(msgReportIn)),
		RequestUAddr:    unsafe.Pointer(&msgReportIn),
		ResponseLength:  uint16(unsafe.Sizeof(msgReportOut)),
		ResponseUAddr:   unsafe.Pointer(&msgReportOut),
		Error:           0,
	}

	if err := linux.Ioctl(f, snpGetReportIoctlCode5, unsafe.Pointer(payload)); err != nil {
		return nil, err
	}
	return msgReportOut.Report[:], nil
}

func fetchRawSNPReport6(reportData []byte) ([]byte, error) {
	f, err := os.OpenFile(snpDevicePath6, os.O_RDWR, 0)
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
	reportOutContainer := [reportResponseContainerLength6]byte{}

	if reportData != nil {
		if len(reportData) > len(msgReportIn.ReportData) {
			return nil, fmt.Errorf("reportData too large: %s", reportData)
		}
		copy(msgReportIn.ReportData[:], reportData)
	}

	payload := &guestRequest6{
		MsgVersion:   1,
		RequestData:  unsafe.Pointer(&msgReportIn),
		ResponseData: unsafe.Pointer(&reportOutContainer),
		Error:        0,
	}

	if err := linux.Ioctl(f, snpGetReportIoctlCode6, unsafe.Pointer(payload)); err != nil {
		return nil, err
	}

	msgReportOut = *(*reportResponse)(unsafe.Pointer(&reportOutContainer[0]))

	return msgReportOut.Report[:], nil
}

func CheckDriverError() error {
	return nil
}
