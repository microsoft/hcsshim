//go:build windows

package remotevm

import (
	"github.com/pkg/errors"
)

func (uvmb *utilityVMBuilder) SetStorageQos(iopsMaximum int64, bandwidthMaximum int64) error {
	// The way that HCS handles these options is a bit odd. They launch the vmworker process in a job object and
	// set the bandwidth and iops limits on the worker process' job object. To keep parity with what we expose today
	// in HCS we can do the same here as we launch the server process in a job object.
	if uvmb.job != nil {
		if err := uvmb.job.SetIOLimit(bandwidthMaximum, iopsMaximum); err != nil {
			return errors.Wrap(err, "failed to set storage qos values on remotevm process")
		}
	}

	return nil
}
