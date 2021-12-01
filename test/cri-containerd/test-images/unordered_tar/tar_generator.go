package main

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

type tarContents struct {
	path string
	body []byte
}

func writeContentsToTar(tw *tar.Writer, contents []tarContents) error {
	for _, file := range contents {
		var hdr *tar.Header
		isDir := (len(file.body) <= 0)
		if isDir {
			hdr = &tar.Header{
				Name: file.path,
				Mode: 0777,
				Size: 0,
			}
		} else {
			hdr = &tar.Header{
				Name: file.path,
				Mode: 0777,
				Size: int64(len(file.body)),
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return errors.Wrapf(err, "failed to write tar header for file: %s", file.path)
		}
		if !isDir {
			if _, err := tw.Write(file.body); err != nil {
				return errors.Wrapf(err, "failed to write contents of file: %s", file.path)
			}
		}
	}
	return nil
}

// createUnorderedTars creates 2 tars (each tar representing a layer of a container image)
// containing unordered file entries inside the `dirPath` directory. Returns the array of
// created tar file paths.  Note: for now we create 2 tar so that we can include unordered
// whiteout entry.  If required this routine can be modified to generate more or less
// tars.
func createUnorderedTars(dirPath string) ([]string, error) {
	layers := [][]tarContents{
		{
			// layer with a few unordered entries
			{"data/", []byte{}},
			{"root.txt", []byte("inside root.txt")},
			{"foo/", []byte{}},
			{"A/B/b.txt", []byte("inside b.txt")},
			{"A/a.txt", []byte("inside a.txt")},
			{"A/", []byte{}},
			{"A/B/", []byte{}},
		},
		{
			// layer with unordered whiteout directory
			{"A/.wh..wh..opq", []byte{}},
			{"foo/xyz.txt", []byte{}},
			{"A/c.txt", []byte("inside a.txt")},
			{"A/", []byte{}},
			{"A/C/", []byte{}},
		},
	}

	generatedTars := []string{}
	for i, layer := range layers {
		layerPath := filepath.Join(dirPath, fmt.Sprintf("tar%d.tar", i+1))
		layerTar, err := os.Create(layerPath)
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed to create tar at path: %s", layerPath)
		}
		defer layerTar.Close()

		tw := tar.NewWriter(layerTar)
		defer tw.Close()
		if err = writeContentsToTar(tw, layer); err != nil {
			return []string{}, errors.Wrapf(err, "failed to write tar contents for tar : %s", layerPath)
		}

		generatedTars = append(generatedTars, layerPath)
	}

	return generatedTars, nil
}
