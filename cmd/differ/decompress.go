//go:build windows

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/images"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/urfave/cli/v2"
)

var decompressCommand = &cli.Command{
	Name:    "decompress",
	Aliases: []string{"decomp", "d"},
	Usage:   fmt.Sprintf("Decompress a %q stream into a %q", images.MediaTypeDockerSchema2LayerGzip, ocispec.MediaTypeImageLayer),
	Action:  actionReExecWrapper(decompress),
}

func decompress(c *cli.Context) error {
	dc, err := compression.DecompressStream(os.Stdin)
	if err != nil {
		return fmt.Errorf("decompress stream creation: %w", err)
	}
	if _, err = io.Copy(os.Stdout, dc); err != nil {
		return fmt.Errorf("io copy to std out: %w", err)
	}
	return nil
}
