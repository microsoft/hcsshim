package vhd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// Converter converts a disk image to and from a VHD format.
type Converter interface {
	ConvertToVHD(f *os.File) error
	ConvertFromVHD(f *os.File) error
}

// FixedVHDConverter converts a disk image to a fixed VHD (not VHDX)
type FixedVHDConverter struct{}

// ConvertToVHD Implementation for converting disk image to a fixed VHD
func (FixedVHDConverter) ConvertToVHD(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		logrus.Infof("[ConvertToVHD] f.Stat failed with %s", err)
		return err
	}

	logrus.Infof("[ConvertToVHD] NewFixedVHDHeader with size = %d", uint64(info.Size()))

	hdr, err := newFixedVHDHeader(uint64(info.Size()))
	if err != nil {
		logrus.Infof("[ConvertToVHD] NewFixedVHDHeader with %s", err)
		return err
	}

	hdrBytes, err := hdr.Bytes()
	if err != nil {
		logrus.Infof("[ConvertToVHD] hdr.Bytes with %s", err)
		return err
	}

	if _, err := f.WriteAt(hdrBytes, info.Size()); err != nil {
		logrus.Infof("[ConvertToVHD] f.WriteAt with %s", err)
		return err
	}
	return nil
}

// ConvertFromVHD converts a fixed VHD to a normal disk image.
func (FixedVHDConverter) ConvertFromVHD(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}

	if info.Size() < fixedVHDHeaderSize {
		return fmt.Errorf("invalid input file: %s", f.Name())
	}

	if err := f.Truncate(info.Size() - fixedVHDHeaderSize); err != nil {
		return err
	}
	return nil
}
