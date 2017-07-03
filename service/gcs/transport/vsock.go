package transport

import (
	"github.com/linuxkit/virtsock/pkg/vsock"
	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/libs/commonutils"
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
	utils.LogMsgf("vsock Dial port (%d)", port)

	conn, err := vsock.Dial(vmaddrCidHost, port)
	if err != nil {
		return nil, errors.Wrap(err, "failed connecting the VsockConnection")
	}
	utils.LogMsgf("vsock Connect port (%d)", port)

	return conn, nil
}
