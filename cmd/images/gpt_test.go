package main

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/ext4/gpt"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/memory"
)

func Test_GPT(t *testing.T) {
	fileReaders := []io.Reader{}
	guids := []guid.GUID{}

	// create numLayers test layers
	numLayers := 5
	for i := 0; i < numLayers; i++ {
		g, err := guid.NewV4()
		if err != nil {
			t.Fatalf("failed to create guid for layer: %v", err)
		}
		guids = append(guids, g)

		name := "test-layer-" + strconv.Itoa(i) + ".tar"
		tmpTarFilePath := filepath.Join(os.TempDir(), name)
		layerTar, err := os.Create(tmpTarFilePath)
		if err != nil {
			t.Fatalf("failed to create output file: %s", err)
		}
		defer os.Remove(tmpTarFilePath)
		fileReaders = append(fileReaders, layerTar)

		tw := tar.NewWriter(layerTar)
		var files = []struct {
			path     string
			typeFlag byte
			linkName string
			body     string
		}{
			{
				path: "/tmp/zzz.txt",
				body: "inside /tmp/zzz.txt",
			},
			{
				path:     "/tmp/xxx.txt",
				linkName: "/tmp/zzz.txt",
				typeFlag: tar.TypeSymlink,
			},
			{
				path:     "/tmp/yyy.txt",
				linkName: "/tmp/xxx.txt",
				typeFlag: tar.TypeLink,
			},
		}
		for _, file := range files {
			hdr := &tar.Header{
				Name:       file.path,
				Typeflag:   file.typeFlag,
				Linkname:   file.linkName,
				Mode:       0777,
				Size:       int64(len(file.body)),
				ModTime:    time.Now(),
				AccessTime: time.Now(),
				ChangeTime: time.Now(),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			if file.body != "" {
				if _, err := tw.Write([]byte(file.body)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		// Go to the beginning of the tar file so that we can read it correctly
		if _, err := layerTar.Seek(0, 0); err != nil {
			t.Fatalf("failed to seek file: %s", err)
		}
	}

	tmpVhdPath := filepath.Join(os.TempDir(), "test-vhd.vhd")
	layerVhd, err := os.Create(tmpVhdPath)
	if err != nil {
		t.Fatalf("failed to create output VHD: %s", err)
	}
	defer os.Remove(tmpVhdPath)

	diskGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("failed to create guid for layer: %v", err)
	}

	if err := createGPTDisk(fileReaders, layerVhd, guids, diskGUID); err != nil {
		t.Fatalf("failed to convert tar to layer vhd: %s", err)
	}

	header, err := gpt.ReadGPTHeader(layerVhd, uint64(gpt.PrimaryHeaderLBA))
	if err != nil {
		t.Fatalf("failed to read header from tar file %v", err)
	}
	if err := gpt.ValidateGPTHeader(&header, diskGUID); err != nil {
		t.Fatalf("gpt header is corrupt: %v", err)
	}

	pmbr, err := gpt.ReadPMBR(layerVhd)
	if err != nil {
		t.Fatal(err)
	}
	if err := gpt.ValidatePMBR(&pmbr); err != nil {
		t.Fatalf("pmbr is corrupt: %v", err)
	}

	altHeader, err := gpt.ReadGPTHeader(layerVhd, header.AlternateLBA)
	if err != nil {
		t.Fatalf("failed to read header from tar file %v", err)
	}
	if err := gpt.ValidateGPTHeader(&altHeader, diskGUID); err != nil {
		t.Fatalf("gpt alt header is corrupt: %v", err)
	}

	_, err = gpt.ReadGPTPartitionArray(layerVhd, header.PartitionEntryLBA, header.NumberOfPartitionEntries)
	if err != nil {
		t.Fatal(err)
	}

	_, err = gpt.ReadGPTPartitionArray(layerVhd, altHeader.PartitionEntryLBA, altHeader.NumberOfPartitionEntries)
	if err != nil {
		t.Fatal(err)
	}

	// check the partition entry checksums
	sizeOfEntryArrayInBytes := gpt.HeaderSizeOfPartitionEntry * header.NumberOfPartitionEntries
	headerEntriesChecksum, err := gpt.CalculateChecksumPartitionEntryArray(layerVhd, uint32(header.PartitionEntryLBA), sizeOfEntryArrayInBytes)
	if err != nil {
		t.Fatalf("failed to calculate expected header entries checksum: %v", err)
	}
	if headerEntriesChecksum != header.PartitionEntryArrayCRC32 {
		t.Fatalf("header partition entry checksum mismatch, expected %v but got %v", headerEntriesChecksum, header.PartitionEntryArrayCRC32)
	}

	altEntriesChecksum, err := gpt.CalculateChecksumPartitionEntryArray(layerVhd, uint32(altHeader.PartitionEntryLBA), sizeOfEntryArrayInBytes)
	if err != nil {
		t.Fatalf("failed to calculate expected alt entries checksum: %v", err)
	}
	if altEntriesChecksum != altHeader.PartitionEntryArrayCRC32 {
		t.Fatalf("alt header partition entry checksum mismatch, expected %v but got %v", altEntriesChecksum, altHeader.PartitionEntryArrayCRC32)
	}

	// check if the resulting vhd is MB aligned
	diskSize, err := layerVhd.Seek(-int64(tar2ext4.VHDFooterSize), io.SeekEnd)
	if err != nil {
		t.Fatalf("failed to seek the size of the disk: %v", err)
	}
	if diskSize%memory.MiB != 0 {
		t.Fatalf("expected the disk to be MB aligned, instead got %v", diskSize)
	}
}

func Test_GPT_NoInputs(t *testing.T) {
	fileReaders := []io.Reader{}
	guids := []guid.GUID{}

	tmpVhdPath := filepath.Join(os.TempDir(), "test-vhd.vhd")
	layerVhd, err := os.Create(tmpVhdPath)
	if err != nil {
		t.Fatalf("failed to create output VHD: %s", err)
	}
	defer os.Remove(tmpVhdPath)

	diskGUID, err := guid.NewV4()
	if err != nil {
		t.Fatalf("failed to create guid for layer: %v", err)
	}

	if err := createGPTDisk(fileReaders, layerVhd, guids, diskGUID); err != nil {
		t.Fatalf("failed to convert tar to layer vhd: %s", err)
	}

	// see if we still produce a valid gpt header
	header, err := gpt.ReadGPTHeader(layerVhd, uint64(gpt.PrimaryHeaderLBA))
	if err != nil {
		t.Fatalf("failed to read header from tar file %v", err)
	}
	if err := gpt.ValidateGPTHeader(&header, diskGUID); err != nil {
		t.Fatalf("gpt header is corrupt: %v", err)
	}

	// see if we still produce a valid pmbr
	pmbr, err := gpt.ReadPMBR(layerVhd)
	if err != nil {
		t.Fatal(err)
	}
	if err := gpt.ValidatePMBR(&pmbr); err != nil {
		t.Fatalf("pmbr is corrupt: %v", err)
	}

	// see if we still produce a valid alt header
	altHeader, err := gpt.ReadGPTHeader(layerVhd, header.AlternateLBA)
	if err != nil {
		t.Fatalf("failed to read header from tar file %v", err)
	}
	if err := gpt.ValidateGPTHeader(&altHeader, diskGUID); err != nil {
		t.Fatalf("gpt alt header is corrupt: %v", err)
	}

	// check entry array size
	if header.NumberOfPartitionEntries != 0 {
		t.Fatalf("expected no header partition entries, instead got %v", header.NumberOfPartitionEntries)
	}
	if altHeader.NumberOfPartitionEntries != 0 {
		t.Fatalf("expected no alt header partition entries, instead got %v", altHeader.NumberOfPartitionEntries)
	}
}
