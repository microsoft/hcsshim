//go:build windows
// +build windows

package cimfs

import (
	"github.com/Microsoft/hcsshim/osversion"
	"github.com/sirupsen/logrus"
)

func IsCimFsSupported() bool {
	rv, err := osversion.BuildRevision()
	if err != nil {
		logrus.WithError(err).Warn("get build revision")
	}
	return osversion.Get().Version == 20348 && rv >= 1700
}
