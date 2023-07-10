package tools

import (
	// import hcsshim so dependencies are synced
	_ "github.com/Microsoft/hcsshim"

	// protobuf/gRPC/ttrpc generation
	_ "github.com/containerd/protobuild"
	_ "github.com/containerd/protobuild/cmd/go-fix-acronym"
	_ "github.com/containerd/ttrpc/cmd/protoc-gen-go-ttrpc"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"

	// used in go:generate directives

	// create syso files for manifesting
	_ "github.com/josephspurrier/goversioninfo/cmd/goversioninfo"

	// generate Win32 API code
	_ "github.com/Microsoft/go-winio/tools/mkwinsyscall"

	// mock gRPC client and servers
	_ "github.com/golang/mock/mockgen"
)
