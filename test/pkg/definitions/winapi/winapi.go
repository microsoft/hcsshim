//go:build windows

package winapi

import (
	internalwinapi "github.com/Microsoft/hcsshim/internal/winapi"
)

var UserNameCharLimit = internalwinapi.UserNameCharLimit
