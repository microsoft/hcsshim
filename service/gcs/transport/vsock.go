package transport

import (
	"fmt"
	"syscall"
	"time"

	"github.com/linuxkit/virtsock/pkg/vsock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	vmaddrCidHost = 2
	vmaddrCidAny  = 0xffffffff
)

// VsockTransport is an implementation of Transport which uses vsock
// sockets.
type VsockTransport struct{}

var _ Transport = &VsockTransport{}

// Dial accepts a vsock socket port number as configuration, and
// returns an unconnected VsockConnection struct.
func (t *VsockTransport) Dial(port uint32) (Connection, error) {
	// HACK: Remove loop when vsock bugs are fixed!
	// Retry 10 times because vsock.Dial can return connection time out
	// due to some underlying kernel bug.
	logrus.Infof("vsock Dial port (%d)", port)
	for i := 0; i < 10; i++ {
		conn, err := vsock.Dial(vmaddrCidHost, port)
		if err == nil {
			logrus.Infof("vsock Connect port (%d)", port)
			return conn, nil
		}
		// If the error was ETIMEDOUT retry, otherwise fail.
		cause := errors.Cause(err)
		if errno, ok := cause.(syscall.Errno); ok && errno == syscall.ETIMEDOUT {
			logrus.Infof("vsock Connect port (%d) timed out, re-trying", port)
			time.Sleep(100 * time.Millisecond)
			continue
		} else {
			return nil, errors.Wrapf(err, "vsock Dial port (%d) failed", port)
		}
	}
	return nil, fmt.Errorf("failed connecting the VsockConnection: can't connect after 10 attempts")
}
