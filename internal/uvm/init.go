package uvm

import (
	"os"
	"strconv"
	"time"
)

func init() {
	envTimeout := os.Getenv("HCSSHIM_TIMEOUT_SECONDS")
	if len(envTimeout) > 0 {
		e, err := strconv.Atoi(envTimeout)
		if err == nil && e > 0 {
			defaultTimeoutSeconds = time.Second * time.Duration(e)
		}
	}
}
