//go:build windows

package hns

import (
	"github.com/Microsoft/hcsshim/hns/internal"
)

type HNSGlobals = internal.HNSGlobals
type HNSVersion = internal.HNSVersion

var (
	HNSVersion1803 = internal.HNSVersion1803
)

func GetHNSGlobals() (*HNSGlobals, error) {
	return internal.GetHNSGlobals()
}
