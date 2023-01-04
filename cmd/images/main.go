package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/hcsshim/ext4/gpt"
	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/memory"
	cli "github.com/urfave/cli/v2"
)

const (
	// flags for gpt disks
	inputFlag              = "input"
	outputFlag             = "output"
	pmbrFlag               = "pmbr"
	headerFlag             = "header"
	altHeaderFlag          = "alt-header"
	entryFlag              = "entry"
	altEntryFlag           = "alt-entry"
	readAllBytesFlag       = "all"
	partitionFlag          = "partition"
	paritionSuperblockFlag = "partition-sb"
)

func main() {
	app := cli.NewApp()
	app.Name = "images"
	app.Usage = "tool for interacting with OCI images"
	app.Commands = []*cli.Command{
		convertToGPT,
		readGPTCommand,
	}
	app.Flags = []cli.Flag{}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var convertToGPT = &cli.Command{
	Name:  "convert-gpt",
	Usage: "converts a set of tar files into a single GPT image",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:     "input",
			Aliases:  []string{"i"},
			Usage:    "input tar files to be converted",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:     "output",
			Aliases:  []string{"o"},
			Usage:    "output file to write gpt disk to",
			Required: true,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		fileNames := cliCtx.StringSlice(inputFlag)
		inputReaders := []io.Reader{}
		if len(fileNames) == 0 {
			return errors.New("input files are required")
		}

		partitionGUIDs := []guid.GUID{}
		for _, p := range fileNames {
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			inputReaders = append(inputReaders, in)

			pGUID, err := guid.NewV4()
			if err != nil {
				return err
			}
			partitionGUIDs = append(partitionGUIDs, pGUID)
		}

		outputName := cliCtx.String(outputFlag)
		if outputName == "" {
			return errors.New("output file is required")
		}

		out, err := os.Create(outputName)
		if err != nil {
			return err
		}

		diskGUID, err := guid.NewV4()
		if err != nil {
			return err
		}

		return createGPTDisk(inputReaders, out, partitionGUIDs, diskGUID)
	},
}

func createGPTDisk(multipleReaders []io.Reader, w io.ReadWriteSeeker, partitionGUIDs []guid.GUID, diskGUID guid.GUID) error {
	// check that we have valid inputs
	if len(partitionGUIDs) != len(multipleReaders) {
		return fmt.Errorf("must supply a single guid for every input file")
	}
	if len(multipleReaders) > gpt.MaxPartitions {
		return fmt.Errorf("readers exceeds max number of partitions for a GPT disk: %d", len(multipleReaders))
	}

	actualSizeOfEntryArrayBytes := gpt.SizeOfPartitionEntry * len(multipleReaders)
	sizeOfEntryArrayBytes := gpt.GetSizeOfEntryArray(len(multipleReaders))

	// find the first useable LBA and corresponding byte
	firstUseableLBA := gpt.GetFirstUseableLBA(sizeOfEntryArrayBytes)
	firstUseableByte := firstUseableLBA * gpt.BlockSizeLogical
	if _, err := w.Seek(int64(firstUseableByte), io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to the first useable LBA in disk with %v", err)
	}

	// prepare to construct the partition entries
	entries := make([]gpt.PartitionEntry, len(multipleReaders))
	startLBA := firstUseableLBA
	endingLBA := firstUseableLBA

	// create partition entires from input readers
	for i, r := range multipleReaders {
		entryGUID := partitionGUIDs[i]

		seekStartByte := startLBA * gpt.BlockSizeLogical
		_, err := w.Seek(int64(seekStartByte), io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek file: %v", err)
		}
		startOffset := int64(seekStartByte)
		currentConvertOpts := []tar2ext4.Option{
			tar2ext4.ConvertWhiteout,
			// tar2ext4.InlineData,
			tar2ext4.StartWritePosition(startOffset),
		}
		bytesWritten, err := tar2ext4.ConvertTarToExt4(r, w, currentConvertOpts...)
		if err != nil {
			return err
		}

		endingLBA = gpt.FindNextUnusedLogicalBlock(seekStartByte+uint64(bytesWritten)) - 1
		entry := gpt.PartitionEntry{
			PartitionTypeGUID:   gpt.LinuxFilesystemDataGUID,
			UniquePartitionGUID: entryGUID,
			StartingLBA:         startLBA,
			EndingLBA:           endingLBA, // inclusive
			Attributes:          0,
			PartitionName:       [72]byte{}, // Ignore partition name
		}
		entries[i] = entry

		// update the startLBA for the next entry
		startLBA = uint64(endingLBA) + 1
	}
	lastUseableLBA := endingLBA
	lastUsedByte := (lastUseableLBA + 1) * gpt.BlockSizeLogical // add 1 to account for bytes within the last used block

	altEntriesArrayStartLBA := gpt.FindNextUnusedLogicalBlock(uint64(lastUsedByte))
	altEntriesArrayStart := altEntriesArrayStartLBA * gpt.BlockSizeLogical

	_, err := w.Seek(int64(altEntriesArrayStart), io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek file: %v", err)
	}

	for _, e := range entries {
		if err := binary.Write(w, binary.LittleEndian, e); err != nil {
			return fmt.Errorf("failed to write backup entry array with: %v", err)
		}
	}

	sizeAfterBackupEntryArrayInBytes, err := w.Seek(int64(altEntriesArrayStart+uint64(sizeOfEntryArrayBytes)), io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek file: %v", err)
	}

	alternateHeaderLBA := gpt.FindNextUnusedLogicalBlock(uint64(sizeAfterBackupEntryArrayInBytes))
	alternateHeaderInBytes := alternateHeaderLBA * gpt.BlockSizeLogical
	_, err = w.Seek(int64(alternateHeaderInBytes), io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek file: %v", err)
	}

	// only calculate the checksum for the actual partition array, do not include reserved bytes
	altEntriesCheckSum, err := gpt.CalculateChecksumPartitionEntryArray(w, uint32(altEntriesArrayStartLBA), uint32(actualSizeOfEntryArrayBytes))
	if err != nil {
		return err
	}
	altGPTHeader := gpt.Header{
		Signature:                gpt.HeaderSignature,
		Revision:                 gpt.HeaderRevision,
		HeaderSize:               gpt.HeaderSize,
		HeaderCRC32:              0, // set to 0 then calculate crc32 checksum and replace
		ReservedMiddle:           0,
		MyLBA:                    alternateHeaderLBA, // LBA of this header
		AlternateLBA:             uint64(gpt.PrimaryHeaderLBA),
		FirstUsableLBA:           firstUseableLBA,
		LastUsableLBA:            lastUseableLBA,
		DiskGUID:                 diskGUID,
		PartitionEntryLBA:        altEntriesArrayStartLBA, // right before this header
		NumberOfPartitionEntries: uint32(len(multipleReaders)),
		SizeOfPartitionEntry:     gpt.HeaderSizeOfPartitionEntry,
		PartitionEntryArrayCRC32: altEntriesCheckSum,
		ReservedEnd:              [420]byte{},
	}
	altGPTHeader.HeaderCRC32, err = gpt.CalculateHeaderChecksum(altGPTHeader)
	if err != nil {
		return err
	}

	// write the alternate header
	if err := binary.Write(w, binary.LittleEndian, altGPTHeader); err != nil {
		return fmt.Errorf("failed to write backup header with: %v", err)
	}

	// write the protectiveMBR at the beginning of the disk
	_, err = w.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek file: %v", err)
	}
	pMBR := gpt.ProtectiveMBR{
		BootCode:               [440]byte{},
		UniqueMBRDiskSignature: 0,
		Unknown:                0,
		PartitionRecord:        [4]gpt.PartitionMBR{},
		Signature:              gpt.ProtectiveMBRSignature,
	}

	// See gpt package for more information on the max value of the size in LBA
	sizeInLBA := uint32(alternateHeaderLBA)
	if alternateHeaderLBA >= uint64(gpt.ProtectiveMBRSizeInLBAMaxValue) {
		sizeInLBA = uint32(gpt.ProtectiveMBRSizeInLBAMaxValue)
	}

	pMBR.PartitionRecord[0] = gpt.PartitionMBR{
		BootIndicator: 0,
		StartingCHS:   gpt.ProtectiveMBRStartingCHS,
		OSType:        gpt.ProtectiveMBRTypeOS,
		EndingCHS:     gpt.CalculateEndingCHS(sizeInLBA),
		StartingLBA:   gpt.PrimaryHeaderLBA, // LBA of the GPT header
		SizeInLBA:     sizeInLBA,            // size of disk minus one is the alternate header LBA
	}

	// write the protectiveMBR
	if err := binary.Write(w, binary.LittleEndian, pMBR); err != nil {
		return fmt.Errorf("failed to write backup header with: %v", err)
	}

	// write partition entries
	_, err = w.Seek(int64(gpt.PrimaryEntryArrayLBA*gpt.BlockSizeLogical), io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek file: %v", err)
	}

	for _, e := range entries {
		if err := binary.Write(w, binary.LittleEndian, e); err != nil {
			return fmt.Errorf("failed to write backup entry array with: %v", err)
		}
	}

	// only calculate the checksum for the actual partition array, do not include reserved bytes if any
	entriesCheckSum, err := gpt.CalculateChecksumPartitionEntryArray(w, gpt.PrimaryEntryArrayLBA, uint32(actualSizeOfEntryArrayBytes))
	if err != nil {
		return err
	}

	// write primary gpt header
	_, err = w.Seek(int64(gpt.PrimaryHeaderLBA*gpt.BlockSizeLogical), io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek file: %v", err)
	}

	hGPT := gpt.Header{
		Signature:                gpt.HeaderSignature,
		Revision:                 gpt.HeaderRevision,
		HeaderSize:               gpt.HeaderSize,
		HeaderCRC32:              0, // set to 0 then calculate crc32 checksum and replace
		ReservedMiddle:           0,
		MyLBA:                    uint64(gpt.PrimaryHeaderLBA), // LBA of this header
		AlternateLBA:             alternateHeaderLBA,
		FirstUsableLBA:           firstUseableLBA,
		LastUsableLBA:            lastUseableLBA,
		DiskGUID:                 diskGUID,
		PartitionEntryLBA:        uint64(gpt.PrimaryEntryArrayLBA), // right after this header
		NumberOfPartitionEntries: uint32(len(multipleReaders)),
		SizeOfPartitionEntry:     gpt.HeaderSizeOfPartitionEntry,
		PartitionEntryArrayCRC32: entriesCheckSum,
		ReservedEnd:              [420]byte{},
	}
	hGPT.HeaderCRC32, err = gpt.CalculateHeaderChecksum(hGPT)
	if err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, hGPT); err != nil {
		return fmt.Errorf("failed to write backup header with: %v", err)
	}

	// align to MB
	diskSize, err := w.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if diskSize%memory.MiB != 0 {
		remainder := memory.MiB - (diskSize % memory.MiB)
		// seek the number of zeros needed to make this disk MB aligned
		diskSize, err = w.Seek(remainder, io.SeekCurrent)
		if err != nil {
			return err
		}
	}

	// convert to VHD with aligned disk size
	if err := tar2ext4.ConvertToVhdWithSize(w, diskSize); err != nil {
		return err
	}
	return nil
}

var readGPTCommand = &cli.Command{
	Name:  "read-gpt",
	Usage: "read various data structures from a gpt disk",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  inputFlag + ",i",
			Usage: "input file to read",
		},
		&cli.BoolFlag{
			Name:  pmbrFlag,
			Usage: "read pmbr data structures",
		},
		&cli.BoolFlag{
			Name:  headerFlag,
			Usage: "read gpt header",
		},
		&cli.BoolFlag{
			Name:  altHeaderFlag,
			Usage: "read alt gpt header",
		},
		&cli.BoolFlag{
			Name:  entryFlag,
			Usage: "read the main entry array",
		},
		&cli.BoolFlag{
			Name:  altEntryFlag,
			Usage: "read the alt entry array",
		},
		&cli.BoolFlag{
			Name:  readAllBytesFlag,
			Usage: "read all bytes",
		},
		&cli.Uint64Flag{
			Name:  partitionFlag,
			Usage: "partition to print",
		},
		&cli.Uint64Flag{
			Name:  paritionSuperblockFlag,
			Usage: "superblock of partition to print",
		},
	},
	Action: func(cliCtx *cli.Context) error {
		ctx := context.Background()
		input := cliCtx.String(inputFlag)
		if input == "" {
			return fmt.Errorf("input must be specified ")
		}
		inFile, err := os.Open(input)
		if err != nil {
			return err
		}

		if cliCtx.Bool(pmbrFlag) {
			pmbr, err := gpt.ReadPMBR(inFile)
			if err != nil {
				return err
			}
			log.G(ctx).WithField(pmbrFlag, pmbr).Info("file pmbr")
		}

		if cliCtx.Bool(headerFlag) {
			header, err := gpt.ReadGPTHeader(inFile, 1)
			if err != nil {
				return err
			}
			log.G(ctx).WithField(headerFlag, header).Info("file header")
		}

		if cliCtx.Bool(entryFlag) {
			header, err := gpt.ReadGPTHeader(inFile, 1)
			if err != nil {
				return err
			}
			entry, err := gpt.ReadGPTPartitionArray(inFile, header.PartitionEntryLBA, header.NumberOfPartitionEntries)
			if err != nil {
				return err
			}
			log.G(ctx).WithField(entryFlag, entry).Info("file entry")
		}

		if cliCtx.Bool(altEntryFlag) {
			header, err := gpt.ReadGPTHeader(inFile, 1)
			if err != nil {
				return err
			}
			altHeader, err := gpt.ReadGPTHeader(inFile, header.AlternateLBA)
			if err != nil {
				return err
			}
			entry, err := gpt.ReadGPTPartitionArray(inFile, altHeader.PartitionEntryLBA, altHeader.NumberOfPartitionEntries)
			if err != nil {
				return err
			}
			log.G(ctx).WithField(altEntryFlag, entry).Info("file alt-entry")
		}

		if cliCtx.Bool(altHeaderFlag) {
			header, err := gpt.ReadGPTHeader(inFile, 1)
			if err != nil {
				return err
			}
			altHeader, err := gpt.ReadGPTHeader(inFile, header.AlternateLBA)
			if err != nil {
				return err
			}
			log.G(ctx).WithField(altHeaderFlag, altHeader).Info("file altHeader")
		}

		if cliCtx.Bool(readAllBytesFlag) {
			all, err := io.ReadAll(inFile)
			if err != nil {
				return err
			}
			resultAsString := ""
			for _, b := range all {
				resultAsString += fmt.Sprintf(" %x", b)
			}
			log.G(ctx).WithField("byte", resultAsString).Infof("all file content in bytes")
		}

		partitionIndex := cliCtx.Uint64(partitionFlag)
		if partitionIndex != 0 {
			header, err := gpt.ReadGPTHeader(inFile, 1)
			if err != nil {
				return err
			}
			entry, err := gpt.ReadGPTPartitionArray(inFile, header.PartitionEntryLBA, header.NumberOfPartitionEntries)
			if err != nil {
				return err
			}
			partitionContent, err := gpt.ReadPartitionRaw(inFile, entry[partitionIndex-1].StartingLBA, entry[partitionIndex-1].EndingLBA)
			if err != nil {
				return err
			}
			log.G(ctx).WithField(partitionFlag, partitionContent).Infof("content of partition %v", partitionIndex)
		}

		partitionSuperblockIndex := cliCtx.Uint64(paritionSuperblockFlag)
		if partitionSuperblockIndex != 0 {
			header, err := gpt.ReadGPTHeader(inFile, 1)
			if err != nil {
				return err
			}
			entry, err := gpt.ReadGPTPartitionArray(inFile, header.PartitionEntryLBA, header.NumberOfPartitionEntries)
			if err != nil {
				return err
			}
			sb, err := tar2ext4.ReadExt4SuperBlockFromPartition(inFile.Name(), int64(entry[partitionSuperblockIndex-1].StartingLBA))
			if err != nil {
				return err
			}
			log.G(ctx).WithField(paritionSuperblockFlag, sb).Infof("content of partition %v", partitionSuperblockIndex)
		}
		return nil
	},
}
