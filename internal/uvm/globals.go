package uvm

import "time"

// defaultTimeoutSeconds is the default time to wait for various operations.
// - Waiting for async notifications from HCS
// - Waiting for processes to launch through
// - Waiting to copy data to/from a launched processes stdio pipes.
// This can be overridden through HCS_TIMEOUT_SECONDS
// TODO This should move to the top-level lcow package after moving the process stuff
var defaultTimeoutSeconds = time.Second * 60 * 4
