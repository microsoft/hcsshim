package uvm

import (
	"os"
	"strconv"
	"time"
)

// defaultTimeoutSeconds is the default time to wait for various operations.
// - Waiting for async notifications from HCS
// - Waiting for processes to launch through
// - Waiting to copy data to/from a launched processes stdio pipes.
// This can be overridden through HCS_TIMEOUT_SECONDS
var defaultTimeoutSeconds = time.Second * 60 * 4

func init() {
	envTimeout := os.Getenv("HCSSHIM_TIMEOUT_SECONDS")
	if len(envTimeout) > 0 {
		e, err := strconv.Atoi(envTimeout)
		if err == nil && e > 0 {
			defaultTimeoutSeconds = time.Second * time.Duration(e)
		}
	}
}
