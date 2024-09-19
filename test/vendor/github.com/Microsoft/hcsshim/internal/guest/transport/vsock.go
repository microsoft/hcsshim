//go:build linux
// +build linux

package transport

import (
	gerrors "errors"
	"fmt"
	"syscall"
	"time"

	"github.com/linuxkit/virtsock/pkg/vsock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// VsockTransport is an implementation of Transport which uses vsock
// sockets.
type VsockTransport struct{}

var _ Transport = &VsockTransport{}

// Dial accepts a vsock socket port number as configuration, and
// returns an unconnected VsockConnection struct.
func (t *VsockTransport) Dial(port uint32) (Connection, error) {
	logrus.WithFields(logrus.Fields{
		"port": port,
	}).Info("opengcs::VsockTransport::Dial - vsock dial port")

	// HACK: Remove loop when vsock bugs are fixed!
	// Retry 10 times because vsock.Dial can return connection time out
	// due to some underlying kernel bug.
	for i := 0; i < 10; i++ {
		conn, err := vsock.Dial(vsock.CIDHost, port)
		if err == nil {
			return conn, nil
		}
		// If the error was ETIMEDOUT retry, otherwise fail.
		var errno syscall.Errno
		if gerrors.As(err, &errno) && errno == syscall.ETIMEDOUT {
			time.Sleep(100 * time.Millisecond)
			continue
		} else {
			return nil, errors.Wrapf(err, "vsock Dial port (%d) failed", port)
		}
	}
	return nil, fmt.Errorf("failed connecting the VsockConnection: can't connect after 10 attempts")
}
