//go:build windows

package shimdiag

import (
	internalshimdiag "github.com/Microsoft/hcsshim/internal/shimdiag"
)

var (
	GetShim           = internalshimdiag.GetShim
	NewShimDiagClient = internalshimdiag.NewShimDiagClient
)

type ExecProcessRequest = internalshimdiag.ExecProcessRequest
type ShareRequest = internalshimdiag.ShareRequest
type ShimDiagService = internalshimdiag.ShimDiagService
