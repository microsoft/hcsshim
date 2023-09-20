//go:build windows

package cmd

import (
	internalcmd "github.com/Microsoft/hcsshim/internal/cmd"
)

var CreatePipeAndListen = internalcmd.CreatePipeAndListen
