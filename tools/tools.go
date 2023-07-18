//go:build tools

package tools

import (
	// for go generate directives

	// generate Win32 API code
	_ "github.com/Microsoft/go-winio/tools/mkwinsyscall"

	// create syso files for manifesting
	_ "github.com/josephspurrier/goversioninfo/cmd/goversioninfo"

	// mock gRPC client and servers
	_ "github.com/golang/mock/mockgen"
)
