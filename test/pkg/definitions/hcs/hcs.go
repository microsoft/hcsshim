//go:build windows

package hcs

import (
	internalhcs "github.com/Microsoft/hcsshim/internal/hcs"
)

var (
	ErrOperationDenied = internalhcs.ErrOperationDenied
	CreateNTFSVHD      = internalhcs.CreateNTFSVHD
)
