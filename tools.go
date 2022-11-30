//go:build tools

package hcsshim

import (
	// protobuf generation
	_ "github.com/containerd/containerd/cmd/protoc-gen-gogoctrd"
	_ "github.com/containerd/protobuild"
	// go generate
	_ "github.com/Microsoft/go-winio/tools/mkwinsyscall"
	_ "github.com/josephspurrier/goversioninfo/cmd/goversioninfo"
)
