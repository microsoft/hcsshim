//go:build windows

package lcow

import (
	internallcow "github.com/Microsoft/hcsshim/internal/lcow"
)

var CreateScratch = internallcow.CreateScratch
