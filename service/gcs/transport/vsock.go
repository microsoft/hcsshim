package transport

import (
	"github.com/Sirupsen/logrus"
	"github.com/linuxkit/virtsock/pkg/vsock"
	"github.com/pkg/errors"
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
	logrus.Infof("vsock Dial port (%d)", port)

	conn, err := vsock.Dial(vmaddrCidHost, port)
	if err != nil {
		return nil, errors.Wrap(err, "failed connecting the VsockConnection")
	}
	logrus.Infof("vsock Connect port (%d)", port)

	return conn, nil
}
