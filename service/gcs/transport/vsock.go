package transport

import (
	"fmt"
	"time"

	"github.com/linuxkit/virtsock/pkg/vsock"
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
	for i := 0; i < 10; i++ {
		logrus.Infof("vsock Dial port (%d)", port)
		conn, err := vsock.Dial(vmaddrCidHost, port)
		if err == nil {
			logrus.Infof("vsock Connect port (%d)", port)
			return conn, nil
		}

		// The virtsock wrapper eats up the syscall error, so we can't distinguish ETIMEDOUT from
		// other errors, so just sleep and try again
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed connecting the VsockConnection: can't connect after 10 attempts")
}
