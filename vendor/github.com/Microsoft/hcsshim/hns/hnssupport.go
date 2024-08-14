//go:build windows

package hns

import (
	"github.com/Microsoft/hcsshim/hns/internal"
)

type HNSSupportedFeatures = internal.HNSSupportedFeatures

type HNSAclFeatures = internal.HNSAclFeatures

func GetHNSSupportedFeatures() HNSSupportedFeatures {
	return internal.GetHNSSupportedFeatures()
}
