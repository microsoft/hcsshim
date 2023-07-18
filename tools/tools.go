//go:build tools

package tools

import (
	// protobuf/gRPC/ttrpc generation
	_ "github.com/containerd/protobuild"
	_ "github.com/containerd/protobuild/cmd/go-fix-acronym"
	_ "github.com/containerd/ttrpc/cmd/protoc-gen-go-ttrpc"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"

	// used in go:generate directives

	// generate Win32 API code
	_ "github.com/Microsoft/go-winio/tools/mkwinsyscall"

	// create syso files for manifesting
	_ "github.com/josephspurrier/goversioninfo/cmd/goversioninfo"

	// mock gRPC client and servers
	_ "go.uber.org/mock/mockgen"
)
