package amdsevsnp

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
)

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
