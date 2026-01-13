//go:build windows

package winapi

import (
	"github.com/Microsoft/go-winio/pkg/guid"

	"github.com/Microsoft/hcsshim/internal/winapi/cimfs"
	"github.com/Microsoft/hcsshim/internal/winapi/cimwriter"
	"github.com/Microsoft/hcsshim/internal/winapi/types"
)

type g = guid.GUID

func CimMountImage(imagePath string, fsName string, flags uint32, volumeID *guid.GUID) error {
	return cimfs.CimMountImage(imagePath, fsName, flags, volumeID)
}

func CimDismountImage(volumeID *guid.GUID) error {
	return cimfs.CimDismountImage(volumeID)
}

func CimCreateImage(imagePath string, oldFSName *uint16, newFSName *uint16, cimFSHandle *types.FsHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCreateImage(imagePath, oldFSName, newFSName, cimFSHandle)
	}
	return cimfs.CimCreateImage(imagePath, oldFSName, newFSName, cimFSHandle)
}

func CimCreateImage2(imagePath string, flags uint32, oldFSName *uint16, newFSName *uint16, cimFSHandle *types.FsHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCreateImage2(imagePath, flags, oldFSName, newFSName, cimFSHandle)
	}
	return cimfs.CimCreateImage2(imagePath, flags, oldFSName, newFSName, cimFSHandle)
}

func CimCloseImage(cimFSHandle types.FsHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCloseImage(cimFSHandle)
	}
	return cimfs.CimCloseImage(cimFSHandle)
}

func CimCommitImage(cimFSHandle types.FsHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCommitImage(cimFSHandle)
	}
	return cimfs.CimCommitImage(cimFSHandle)
}

func CimCreateFile(cimFSHandle types.FsHandle, path string, file *types.CimFsFileMetadata, cimStreamHandle *types.StreamHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCreateFile(cimFSHandle, path, file, cimStreamHandle)
	}
	return cimfs.CimCreateFile(cimFSHandle, path, file, cimStreamHandle)
}

func CimCloseStream(cimStreamHandle types.StreamHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCloseStream(cimStreamHandle)
	}
	return cimfs.CimCloseStream(cimStreamHandle)
}

func CimWriteStream(cimStreamHandle types.StreamHandle, buffer uintptr, bufferSize uint32) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimWriteStream(cimStreamHandle, buffer, bufferSize)
	}
	return cimfs.CimWriteStream(cimStreamHandle, buffer, bufferSize)
}

func CimDeletePath(cimFSHandle types.FsHandle, path string) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimDeletePath(cimFSHandle, path)
	}
	return cimfs.CimDeletePath(cimFSHandle, path)
}

func CimCreateHardLink(cimFSHandle types.FsHandle, newPath string, oldPath string) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCreateHardLink(cimFSHandle, newPath, oldPath)
	}
	return cimfs.CimCreateHardLink(cimFSHandle, newPath, oldPath)
}

func CimCreateAlternateStream(cimFSHandle types.FsHandle, path string, size uint64, cimStreamHandle *types.StreamHandle) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCreateAlternateStream(cimFSHandle, path, size, cimStreamHandle)
	}
	return cimfs.CimCreateAlternateStream(cimFSHandle, path, size, cimStreamHandle)
}

func CimAddFsToMergedImage(cimFSHandle types.FsHandle, path string) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimAddFsToMergedImage(cimFSHandle, path)
	}
	return cimfs.CimAddFsToMergedImage(cimFSHandle, path)
}

func CimAddFsToMergedImage2(cimFSHandle types.FsHandle, path string, flags uint32) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimAddFsToMergedImage2(cimFSHandle, path, flags)
	}
	return cimfs.CimAddFsToMergedImage2(cimFSHandle, path, flags)
}

func CimMergeMountImage(numCimPaths uint32, backingImagePaths *types.CimFsImagePath, flags uint32, volumeID *guid.GUID) error {
	return cimfs.CimMergeMountImage(numCimPaths, backingImagePaths, flags, volumeID)
}

func CimTombstoneFile(cimFSHandle types.FsHandle, path string) error {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimTombstoneFile(cimFSHandle, path)
	}
	return cimfs.CimTombstoneFile(cimFSHandle, path)
}

func CimCreateMergeLink(cimFSHandle types.FsHandle, newPath string, oldPath string) (hr error) {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimCreateMergeLink(cimFSHandle, newPath, oldPath)
	}
	return cimfs.CimCreateMergeLink(cimFSHandle, newPath, oldPath)
}

func CimSealImage(blockCimPath string, hashSize *uint64, fixedHeaderSize *uint64, hash *byte) (hr error) {
	if cimwriter.CimWriterSupported() {
		return cimwriter.CimSealImage(blockCimPath, hashSize, fixedHeaderSize, hash)
	}
	return cimfs.CimSealImage(blockCimPath, hashSize, fixedHeaderSize, hash)
}

func CimGetVerificationInformation(blockCimPath string, isSealed *uint32, hashSize *uint64, signatureSize *uint64, fixedHeaderSize *uint64, hash *byte, signature *byte) (hr error) {
	return cimfs.CimGetVerificationInformation(blockCimPath, isSealed, hashSize, signatureSize, fixedHeaderSize, hash, signature)
}

func CimMountVerifiedImage(imagePath string, fsName string, flags uint32, volumeID *guid.GUID, hashSize uint16, hash *byte) error {
	return cimfs.CimMountVerifiedImage(imagePath, fsName, flags, volumeID, hashSize, hash)
}

func CimMergeMountVerifiedImage(numCimPaths uint32, backingImagePaths *types.CimFsImagePath, flags uint32, volumeID *guid.GUID, hashSize uint16, hash *byte) error {
	return cimfs.CimMergeMountVerifiedImage(numCimPaths, backingImagePaths, flags, volumeID, hashSize, hash)
}
