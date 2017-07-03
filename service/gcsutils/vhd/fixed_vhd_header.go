package vhd

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// Constants for the VHD header
const CookieMagic = 0x636f6e6563746978 // "conectix" as 64 bit big endian number
const FeatureMask = 0x2
const FileFormatVersionMagic = 0x00010000
const FixedDataOffset = 0xFFFFFFFFFFFFFFFF
const CreatorApplicationMagic = 0x77696e20 // "win " as 4 bit big endian number. It's win<SPACE>
const CreatorVersionMagic = 0x000a0000
const CreatorHostOSMagic = 0x5769326b // "wi2k" as 4 bit big endian number

// DiskType Constants
// DiskType = 1, 5, 6 are deprecated
const DiskTypeNone = 0
const DiskTypeFixed = 2
const DiskTypeDynamic = 3
const DiskTypeDifferencing = 4

// Saved state constants
const SaveStateYes = 1
const SaveStateNo = 0

// Consts for the CHS calculation
const sectorBytes = 512
const cMax = 65535
const hMax = 16
const sMax = 255
const MaxCHS = cMax * hMax * sMax * sectorBytes

// Variables for time stamp
const TimeStart = "00 Jan 01 00:00 UTC"

var oldTime time.Time

// FixedVHDHeader is the footer for a fixed VHD
type FixedVHDHeader struct {
	Cookie             uint64
	Features           uint32
	FileFormatVersion  uint32
	DataOffset         uint64
	TimeStamp          uint32
	CreatorApplication uint32
	CreatorVersion     uint32
	CreatorHostOS      uint32
	OriginalSize       uint64
	CurrentSize        uint64
	DiskGeometry       uint32
	DiskType           uint32
	Checksum           uint32
	UniqueID           [16]uint8
	SavedState         uint8
	Reserved           [427]uint8
}

const FixedVHDHeaderSize int64 = 512

func init() {
	oldTime, _ = time.Parse(time.RFC822, TimeStart)
}

func NewFixedVHDHeader(size uint64) (*FixedVHDHeader, error) {
	header := newBasicFixedVHDHeader()

	timestamp := calculateTimeStamp()
	header.TimeStamp = timestamp

	header.OriginalSize = size
	header.CurrentSize = size

	chs, err := calculateCHS(size)
	if err != nil {
		return nil, err
	}
	header.DiskGeometry = chs

	header.DiskType = DiskTypeFixed

	id, err := generateUUID()
	if err != nil {
		return nil, err
	}
	header.UniqueID = id

	chk, err := calculateCheckSum(header)
	if err != nil {
		return nil, err
	}
	header.Checksum = chk
	return header, nil
}

func (hdr *FixedVHDHeader) Bytes() ([]byte, error) {
	b := &bytes.Buffer{}
	if err := binary.Write(b, binary.BigEndian, hdr); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func newBasicFixedVHDHeader() *FixedVHDHeader {
	return &FixedVHDHeader{
		Cookie:             CookieMagic,
		Features:           FeatureMask,
		FileFormatVersion:  FileFormatVersionMagic,
		DataOffset:         FixedDataOffset,
		CreatorApplication: CreatorApplicationMagic,
		CreatorVersion:     CreatorVersionMagic,
		CreatorHostOS:      CreatorHostOSMagic,
	}
}

func calculateCHS(totalSize uint64) (uint32, error) {
	if totalSize == 0 || totalSize%sectorBytes != 0 {
		return 0, fmt.Errorf("size must be a non zero multiple of 512")
	}

	totalSectors := totalSize / sectorBytes
	if totalSectors > MaxCHS {
		totalSectors = MaxCHS
	}

	var sectorsPerTrack uint64
	var heads uint64
	var cylinderTimesHeads uint64
	if totalSectors >= cMax*hMax*63 {
		sectorsPerTrack = 255
		heads = 16
		cylinderTimesHeads = totalSectors / sectorsPerTrack
	} else {
		sectorsPerTrack = 17
		cylinderTimesHeads = totalSectors / sectorsPerTrack

		heads = (cylinderTimesHeads + 1023) / 1024
		if heads < 4 {
			heads = 4
		}
		if cylinderTimesHeads >= (heads*1024) || heads >= 16 {
			sectorsPerTrack = 31
			heads = 16
			cylinderTimesHeads = totalSectors / sectorsPerTrack
		}
		if cylinderTimesHeads >= (heads * 1024) {
			sectorsPerTrack = 63
			heads = 16
			cylinderTimesHeads = totalSectors / sectorsPerTrack
		}
	}
	cylinders := cylinderTimesHeads / heads

	// Sanity check
	if cylinders > cMax || heads > hMax || sectorsPerTrack > sMax {
		return 0, fmt.Errorf("invalid size value. Must be less than %d", MaxCHS)
	}

	// Now the values into a single big endian int.
	var res uint32
	res |= uint32(cylinders << 16)
	res |= uint32(heads << 8)
	res |= uint32(sectorsPerTrack)
	return res, nil
}

func calculateCheckSum(header *FixedVHDHeader) (uint32, error) {
	oldchk := header.Checksum
	header.Checksum = 0

	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.BigEndian, header); err != nil {
		return 0, err
	}

	var chk uint32
	bufBytes := buf.Bytes()
	for i := 0; i < len(bufBytes); i++ {
		chk += uint32(bufBytes[i])
	}
	header.Checksum = oldchk
	return uint32(^chk), nil
}

func calculateTimeStamp() uint32 {
	return uint32(time.Since(oldTime).Seconds())
}

func generateUUID() ([16]byte, error) {
	res := [16]byte{}
	if _, err := rand.Read(res[:]); err != nil {
		return res, err
	}
	return res, nil
}
