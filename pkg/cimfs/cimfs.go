//go:build windows
// +build windows

package cimfs

import (
	"fmt"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
)

var (
	ErrMergedCimNotSupported = fmt.Errorf("merged CIMs are not supported on this OS version")
)

func IsCimFSSupported() bool {
	rv, err := osversion.BuildRevision()
	if err != nil {
		logrus.WithError(err).Warn("get build revision")
	}
	return osversion.Build() == 20348 && rv >= 2031
}

func IsMergedCimSupported() bool {
	return osversion.Build() >= 26100
}
