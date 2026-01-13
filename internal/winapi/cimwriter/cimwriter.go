//go:build windows

package cimwriter

import (
	"github.com/Microsoft/hcsshim/internal/winapi/types"
)

type FsHandle = types.FsHandle
type StreamHandle = types.StreamHandle
type FileMetadata = types.CimFsFileMetadata
type ImagePath = types.CimFsImagePath

//sys CimCreateImage(imagePath string, oldFSName *uint16, newFSName *uint16, cimFSHandle *FsHandle) (hr error) = cimwriter.CimCreateImage?
//sys CimCreateImage2(imagePath string, flags uint32, oldFSName *uint16, newFSName *uint16, cimFSHandle *FsHandle) (hr error) = cimwriter.CimCreateImage2?
//sys CimCloseImage(cimFSHandle FsHandle) = cimwriter.CimCloseImage?
//sys CimCommitImage(cimFSHandle FsHandle) (hr error) = cimwriter.CimCommitImage?

//sys CimCreateFile(cimFSHandle FsHandle, path string, file *FileMetadata, cimStreamHandle *StreamHandle) (hr error) = cimwriter.CimCreateFile?
//sys CimCloseStream(cimStreamHandle StreamHandle) (hr error) = cimwriter.CimCloseStream?
//sys CimWriteStream(cimStreamHandle StreamHandle, buffer uintptr, bufferSize uint32) (hr error) = cimwriter.CimWriteStream?
//sys CimDeletePath(cimFSHandle FsHandle, path string) (hr error) = cimwriter.CimDeletePath?
//sys CimCreateHardLink(cimFSHandle FsHandle, newPath string, oldPath string) (hr error) = cimwriter.CimCreateHardLink?
//sys CimCreateAlternateStream(cimFSHandle FsHandle, path string, size uint64, cimStreamHandle *StreamHandle) (hr error) = cimwriter.CimCreateAlternateStream?
//sys CimAddFsToMergedImage(cimFSHandle FsHandle, path string) (hr error) = cimwriter.CimAddFsToMergedImage?
//sys CimAddFsToMergedImage2(cimFSHandle FsHandle, path string, flags uint32) (hr error) = cimwriter.CimAddFsToMergedImage2?

//sys CimTombstoneFile(cimFSHandle FsHandle, path string) (hr error) = cimwriter.CimTombstoneFile?
//sys CimCreateMergeLink(cimFSHandle FsHandle, newPath string, oldPath string) (hr error) = cimwriter.CimCreateMergeLink?
//sys CimSealImage(blockCimPath string, hashSize *uint64, fixedHeaderSize *uint64, hash *byte) (hr error) = cimwriter.CimSealImage?

// CimWriterSupported checks if cimwriter.dll is present on the system.
func CimWriterSupported() bool {
	return modcimwriter.Load() == nil
}
