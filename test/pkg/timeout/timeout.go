// Package provides functionality for timing out operations and waiting
// for deadlines.
package timeout

import (
	"time"
)

// arbitrary timeouts
const (
	// how long to wait for connections to be made
	ConnectTimeout = time.Second * 5

	// how long to wait for an entire operation
	BenchmarkOperationTimeout = time.Second * 16
)
