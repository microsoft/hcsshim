package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

func createBlob(blobsDirPath string, blobContents io.Reader) (int64, digest.Digest, error) {
	tempFile, err := os.CreateTemp(blobsDirPath, "")
	if err != nil {
		return 0, "", errors.Wrapf(err, "failed to create file")
	}
	defer tempFile.Close()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hasher)
	written, err := io.Copy(multiWriter, blobContents)
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to copy content")
	}
	dgst := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	tempFile.Close()

	// name the blob file with its digest (excluding the first `sha256:` part)
	if err = os.Rename(tempFile.Name(), filepath.Join(blobsDirPath, dgst[7:])); err != nil {
		return 0, "", errors.Wrap(err, "renaming content file failed")
	}
	return written, digest.Digest(dgst), nil
}

func createBlobFromTar(tarPath, blobsDirPath string) (int64, digest.Digest, error) {
	srcFile, err := os.Open(tarPath)
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to open content source file")
	}
	defer srcFile.Close()

	return createBlob(blobsDirPath, srcFile)
}

// createBlobFromStruct converts the given struct into json and creates a blob from that.
func createBlobFromStruct(blobsDirPath string, data interface{}) (int64, digest.Digest, error) {
	dataJson, err := json.Marshal(data)
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to marshal struct")
	}

	// copy config
	buf := bytes.NewBuffer(dataJson)
	clen, dgst, err := createBlob(blobsDirPath, buf)
	if err != nil {
		return 0, "", errors.Wrap(err, "config content write failed")
	}
	return clen, dgst, nil
}

// createMinimalConfig creates a very minimal but valid image config based on the image
// config definition given here:
// https://github.com/opencontainers/image-spec/blob/main/config.md.
func createMinimalConfig(layers []ocispec.Descriptor) ocispec.Image {
	var img ocispec.Image
	img.Architecture = "amd64"
	img.OS = "linux"
	img.RootFS.Type = "layers"
	for _, layer := range layers {
		img.RootFS.DiffIDs = append(img.RootFS.DiffIDs, layer.Digest)
	}
	return img
}

func createOciLayoutFile(dirPath string) error {
	// create oci layout file
	flayout, err := os.Create(filepath.Join(dirPath, "oci-layout"))
	if err != nil {
		return errors.Wrap(err, "failed to create oci layout")
	}
	_, err = flayout.Write([]byte(`{"imageLayoutVersion":"1.0.0"}`))
	if err != nil {
		return errors.Wrap(err, "failed to write layout file")
	}
	return nil
}

// createImageFromLayerTars creates an OCI compliant image (from given layer tars) that
// can be imported to containerd. Note that the layers might not even be valid container
// image layers and so there is no guarantee that this image will work for running a
// container. Main purpose of this routine is to create an image containing specific tars
// so that we can import this image with containerd and catch any image extraction bugs.
func createImageFromLayerTars(layerTars []string) error {
	// A very minimal image must contain following things:
	//
	// 1. oci layout file:
	// A file named `oci-layout` situated at the root of the image tar and containing
	// the string `{"imageLayoutVersion":"1.0.0"}`.
	//
	// 2. Index: The index must be a file named `index.json` and must be situated at
	// the root of the image tar. The index should have one descriptor for the image
	// manifest. Note that all of the content referenced here onwards will be named
	// with their sha256 digest value and will be stored under `./blobs/sha256`
	// directory (i.e the image tar should have a blobs/sha256 directory at the root)
	//
	// 3. A manifest:
	// Image manifest is a json file stored under the blobs/sha256 directory. The name
	// of this file is its sha256 digest and this digest will be provided in the
	// descriptor entry in index.json.

	// 4. A config: Image config provides most of the image metadata and the json file
	// representing this config should also be stored under blobs/sha256. The manifest
	// should provide the digest of the config.

	// 5. The image layers: The image layers should be stored under the blobs/sha256
	// directory and the layer tar files should be named with their sha256 digest. The
	// manifest should provide a descriptor for each of the layers.

	// create a directory under which all image files are stored. This directory will be
	// converted into the image tar at the end.
	tempDirPath, err := os.MkdirTemp("", "imagecreator-imagedir")
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory")
	}
	defer os.RemoveAll(tempDirPath)

	sha256dirPath := filepath.Join(tempDirPath, "blobs", "sha256")
	err = os.MkdirAll(sha256dirPath, 0777)
	if err != nil {
		return errors.Wrap(err, "failed to create blobs dir")
	}

	// copy all layer tar as blobs
	layerBlobs := []ocispec.Descriptor{}
	for _, layerTar := range layerTars {
		llen, dgst, err := createBlobFromTar(layerTar, sha256dirPath)
		if err != nil {
			return errors.Wrap(err, "layer content write failed")
		}
		layerBlobs = append(layerBlobs, ocispec.Descriptor{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    dgst,
			Size:      llen,
		})
	}

	// create image config blob
	clen, cdgst, err := createBlobFromStruct(sha256dirPath, createMinimalConfig(layerBlobs))
	if err != nil {
		return errors.Wrap(err, "failed to create config blob")
	}

	// create manifest blob
	var manifest ocispec.Manifest
	manifest.SchemaVersion = 2
	manifest.Config.Size = clen
	manifest.Config.Digest = cdgst
	manifest.Config.MediaType = "application/vnd.docker.container.image.v1+json"
	manifest.Layers = layerBlobs
	mlen, mdgst, err := createBlobFromStruct(sha256dirPath, manifest)
	if err != nil {
		return errors.Wrap(err, "failed to crate blob for manifest")
	}

	// create index file
	var index ocispec.Index
	index.SchemaVersion = 2
	index.Manifests = append(index.Manifests, ocispec.Descriptor{
		MediaType: "application/vnd.docker.distribution.manifest.v2+json",
		Digest:    mdgst,
		Size:      mlen,
	})

	indexJson, err := json.Marshal(index)
	if err != nil {
		return errors.Wrap(err, "failed to marshal index json")
	}

	findex, err := os.Create(filepath.Join(tempDirPath, "index.json"))
	if err != nil {
		return errors.Wrap(err, "failed to create index file")
	}
	_, err = findex.Write(indexJson)
	if err != nil {
		return errors.Wrap(err, "failed to write index.json")
	}

	// create oci layout file
	if err = createOciLayoutFile(tempDirPath); err != nil {
		return err
	}

	// create tar from the image
	tarCmd := exec.Command("tar", "-C", tempDirPath, "-cf", "testimage.tar", ".")
	if err = tarCmd.Run(); err != nil {
		return errors.Wrap(err, "image tar creation failed")
	}
	return nil
}

func main() {
	tempDir, err := os.MkdirTemp("", "imagecreator-layerdir")
	if err != nil {
		log.Fatalf("failed to create temp dir: %s\n", err)
	}
	defer os.RemoveAll(tempDir)

	layerTars, err := createUnorderedTars(tempDir)
	if err != nil {
		log.Fatalf("failed to create unordered tars: %s\n", err)
	}

	if err = createImageFromLayerTars(layerTars); err != nil {
		log.Fatalf("failed to create image: %s\n", err)
	}
}
