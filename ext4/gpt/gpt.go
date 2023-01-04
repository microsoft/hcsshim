package gpt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/ext4/internal/compactext4"
)

func GetSizeOfEntryArray(numEntries int) int {
	sizeOfEntryArrayBytes := SizeOfPartitionEntry * numEntries
	entryArrayReservedSize := ReservedLBAsForParitionEntryArray * BlockSizeLogical
	if sizeOfEntryArrayBytes < entryArrayReservedSize {
		sizeOfEntryArrayBytes = entryArrayReservedSize
	}
	return sizeOfEntryArrayBytes
}

func GetFirstUseableLBA(sizeOfEntryArrayBytes int) uint64 {
	totalGPTInfoSizeInBytes := sizeOfEntryArrayBytes + SizeOfHeaderInBytes
	totaMetadataSizeInBytes := totalGPTInfoSizeInBytes + SizeOfPMBRInBytes

	firstUseableLBA := FindNextUnusedLogicalBlock(uint64(totaMetadataSizeInBytes))
	if firstUseableLBA < MinFirstUsableLBA {
		firstUseableLBA = MinFirstUsableLBA
	}
	return firstUseableLBA
}

func FindNextUnusedLogicalBlock(bytePosition uint64) uint64 {
	block := uint64(bytePosition / BlockSizeLogical)
	if bytePosition%BlockSizeLogical == 0 {
		return block
	}

	return block + 1
}

func CalculateEndingCHS(sizeInLBA uint32) [3]byte {
	// See gpt package for more information on the max value of the ending CHS
	result := [3]byte{}
	if sizeInLBA >= ProtectiveMBREndingCHSMaxValue {
		result = ProtectiveMBREndingCHSMaxArray
	} else {
		// Since sizeInLBA is 1 byte bigger than the CHS array capacity,
		// use a temporary array to hold the result. At this point, we know
		// that the value of sizeInLBA can be represented in the CHS array,
		// so we can safely ignore the last value placed by binary package
		// as it will always be 0x00
		tempResult := [4]byte{}
		binary.LittleEndian.PutUint32(tempResult[:], sizeInLBA)
		copy(result[:], tempResult[:len(tempResult)-1])
	}
	return result
}

func CalculateHeaderChecksum(header Header) (uint32, error) {
	buf := &bytes.Buffer{}
	// do not include reserved field
	if err := binary.Write(buf, binary.LittleEndian, header); err != nil {
		return 0, err
	}

	checksum := crc32.ChecksumIEEE(buf.Bytes()[:HeaderSize])
	return checksum, nil
}

func CalculateChecksumPartitionEntryArray(w io.ReadWriteSeeker, entryArrayLBA uint32, readLengthInBytes uint32) (uint32, error) {
	currentBytePosition, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	// seek to position of entry array
	entryArrayOffsetInBytes := int64(entryArrayLBA * compactext4.BlockSizeLogical)
	_, err = w.Seek(entryArrayOffsetInBytes, io.SeekStart)
	if err != nil {
		return 0, err
	}
	buf := make([]byte, readLengthInBytes)
	if err := binary.Read(w, binary.LittleEndian, buf); err != nil {
		return 0, err
	}

	// calculate crc32 hash
	checksum := crc32.ChecksumIEEE(buf)

	if _, err := w.Seek(currentBytePosition, io.SeekStart); err != nil {
		return 0, err
	}
	return checksum, nil
}

func ReadPMBR(r io.ReadSeeker) (ProtectiveMBR, error) {
	current, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return ProtectiveMBR{}, fmt.Errorf("failed to seek the current byte: %v", err)
	}

	pMBRByteLocation := 0 * compactext4.BlockSizeLogical
	if _, err := r.Seek(int64(pMBRByteLocation), io.SeekStart); err != nil {
		return ProtectiveMBR{}, fmt.Errorf("failed to seek to pMBR byte location %d with: %v", pMBRByteLocation, err)
	}
	pMBR := ProtectiveMBR{}
	if err := binary.Read(r, binary.LittleEndian, &pMBR); err != nil {
		return ProtectiveMBR{}, fmt.Errorf("failed to read pMBR: %v", err)
	}

	if _, err := r.Seek(current, io.SeekStart); err != nil {
		return ProtectiveMBR{}, fmt.Errorf("failed to seek back to current byte position: %v", err)
	}
	return pMBR, nil
}

func ReadGPTHeader(r io.ReadSeeker, lba uint64) (Header, error) {
	current, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return Header{}, fmt.Errorf("failed to seek the current byte: %v", err)
	}

	headerByteLocation := lba * compactext4.BlockSizeLogical
	if _, err := r.Seek(int64(headerByteLocation), io.SeekStart); err != nil {
		return Header{}, fmt.Errorf("failed to seek to header byte location %d with: %v", headerByteLocation, err)
	}

	header := Header{}
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return Header{}, fmt.Errorf("failed to read gpt header: %v", err)
	}
	if _, err := r.Seek(current, io.SeekStart); err != nil {
		return Header{}, fmt.Errorf("failed to seek back to current byte position: %v", err)
	}

	return header, nil
}

func ReadGPTPartitionArray(r io.ReadSeeker, entryArrayLBA uint64, numEntries uint32) ([]PartitionEntry, error) {
	// seek to position of entry array
	currentBytePosition, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	entryArrayOffsetInBytes := int64(entryArrayLBA * compactext4.BlockSizeLogical)
	_, err = r.Seek(entryArrayOffsetInBytes, io.SeekStart)
	if err != nil {
		return nil, err
	}

	entries := make([]PartitionEntry, numEntries)
	for i := 0; i < int(numEntries); i++ {
		entry := PartitionEntry{}
		if err := binary.Read(r, binary.LittleEndian, &entry); err != nil {
			return nil, fmt.Errorf("failed to read entry index %d with: %v", i, err)
		}
		entries[i] = entry
	}

	// set writer back to position we started at
	if _, err := r.Seek(currentBytePosition, io.SeekStart); err != nil {
		return nil, err
	}
	return entries, nil
}

func ReadPartitionRaw(r io.ReadSeeker, partitionLBA, endingLBA uint64) ([]byte, error) {
	// seek to position of entry array
	currentBytePosition, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	partitionOffset := partitionLBA * compactext4.BlockSizeLogical
	endingOffsetEndBlock := (endingLBA * compactext4.BlockSizeLogical) + compactext4.BlockSizeLogical
	if _, err := r.Seek(int64(partitionOffset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to header byte location %d with: %v", partitionOffset, err)
	}

	buf := make([]byte, endingOffsetEndBlock-partitionOffset)
	if err := binary.Read(r, binary.LittleEndian, &buf); err != nil {
		return nil, fmt.Errorf("failed to read gpt header: %v", err)
	}

	// set writer back to position we started at
	if _, err := r.Seek(currentBytePosition, io.SeekStart); err != nil {
		return nil, err
	}
	return buf, nil
}

func ValidateGPTHeader(h *Header, diskGUID guid.GUID) error {
	if h.Signature != HeaderSignature {
		return fmt.Errorf("expected %v for the header signature, instead got %v", HeaderSignature, h.Signature)
	}
	if h.Revision != HeaderRevision {
		return fmt.Errorf("expected %v for the header revision, instead got %v", HeaderRevision, h.Revision)
	}
	if h.HeaderSize != HeaderSize {
		return fmt.Errorf("expected %v for the header size, instead got %v", HeaderSize, h.HeaderSize)
	}
	if h.ReservedMiddle != 0 {
		return fmt.Errorf("expected reserved middle bytes, instead got %v", h.ReservedMiddle)
	}
	if h.DiskGUID != diskGUID {
		return fmt.Errorf("expected to get disk guid of %v, instead got %v", diskGUID, h.DiskGUID.String())
	}
	if h.ReservedEnd != [420]byte{} {
		return fmt.Errorf("expected to find reserved bytes at end of header, instead found %v", h.ReservedEnd)
	}

	// check the header's checksum
	actualHeaderChecksum := h.HeaderCRC32
	h.HeaderCRC32 = 0

	// calculate the expected header checksum
	headerChecksum, err := CalculateHeaderChecksum(*h)
	if err != nil {
		return fmt.Errorf("failed to calculate the header checksum: %v", err)
	}
	if headerChecksum != actualHeaderChecksum {
		return fmt.Errorf("mismatch in calculated checksum, expected to get %v, instead got %v", headerChecksum, actualHeaderChecksum)
	}

	// reset checksum
	h.HeaderCRC32 = actualHeaderChecksum

	return nil
}

func ValidatePMBR(pmbr *ProtectiveMBR) error {
	if pmbr.BootCode != [440]byte{} {
		return fmt.Errorf("expected unused boot code in pmbr, instead found %v", pmbr.BootCode)
	}
	if pmbr.UniqueMBRDiskSignature != 0 {
		return fmt.Errorf("expected field to be set to 0, instead found %v", pmbr.UniqueMBRDiskSignature)
	}
	if pmbr.Unknown != 0 {
		return fmt.Errorf("expected field to be set to 0, instead found %v", pmbr.Unknown)
	}
	if len(pmbr.PartitionRecord) != 4 {
		return fmt.Errorf("expected 4 partition records, instead found %v", len(pmbr.PartitionRecord))
	}
	if pmbr.Signature != ProtectiveMBRSignature {
		return fmt.Errorf("expected pmbr signature to be %v, instead got %v", ProtectiveMBRSignature, pmbr.Signature)
	}

	// check the first partition, which should contain information about the gpt disk
	pr := pmbr.PartitionRecord[0]
	if pr.BootIndicator != 0 {
		return fmt.Errorf("expected partition record's boot indicator to be 0, instead found %v", pr.BootIndicator)
	}
	if pr.StartingCHS != ProtectiveMBRStartingCHS {
		return fmt.Errorf("expected startign CHS to be %v, instead found %v", ProtectiveMBRStartingCHS, pr.StartingCHS)
	}
	if pr.OSType != ProtectiveMBRTypeOS {
		return fmt.Errorf("expected partition record's os type to be %v, instead found %v", ProtectiveMBRTypeOS, pr.OSType)
	}
	if pr.StartingLBA != PrimaryHeaderLBA {
		return fmt.Errorf("expected startign LBA to be 1, instead got %v", pr.StartingLBA)
	}

	expectedEndingCHS := CalculateEndingCHS(pr.SizeInLBA)
	if pr.EndingCHS != expectedEndingCHS {
		return fmt.Errorf("expected ending chs to be %v, instead got %v", expectedEndingCHS, pr.EndingCHS)
	}
	return nil
}
