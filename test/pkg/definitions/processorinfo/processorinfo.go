//go:build windows

package processorinfo

import (
	internalpinfo "github.com/Microsoft/hcsshim/internal/processorinfo"
)

var HostProcessorInfo = internalpinfo.HostProcessorInfo
