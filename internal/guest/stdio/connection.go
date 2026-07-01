//go:build linux
// +build linux

package stdio

import (
	"github.com/pkg/errors"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
)

// ConnectionSettings describe the stdin, stdout, stderr ports to connect the
// transport to. A nil port specifies no connection.
type ConnectionSettings struct {
	StdIn  *uint32
	StdOut *uint32
	StdErr *uint32
}

// Connect returns new transport.Connection instances, one for each stdio pipe
// to be used. If CreateStd*Pipe for a given pipe is false, the given Connection
// is set to nil. Each connection is wrapped in a ConnSlot so the underlying
// vsock can be replaced when the bridge reconnects after live migration.
func Connect(tport transport.Transport, settings ConnectionSettings) (_ *ConnectionSet, err error) {
	connSet := &ConnectionSet{}
	defer func() {
		if err != nil {
			connSet.Close()
		}
	}()
	if settings.StdIn != nil {
		port := *settings.StdIn
		c, err := tport.Dial(port)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdin Connection")
		}
		connSet.In = NewConnSlot(transport.NewLogConnection(c, port), redialer(tport, port))
	}
	if settings.StdOut != nil {
		port := *settings.StdOut
		c, err := tport.Dial(port)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
		connSet.Out = NewConnSlot(transport.NewLogConnection(c, port), redialer(tport, port))
	}
	if settings.StdErr != nil {
		port := *settings.StdErr
		c, err := tport.Dial(port)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
		connSet.Err = NewConnSlot(transport.NewLogConnection(c, port), redialer(tport, port))
	}
	return connSet, nil
}

// redialer returns a callback that re-dials the given vsock port via the
// provided transport. Used by ConnSlot to recover from a bridge disconnect:
// after live migration the source-host listener is gone but the destination
// host has a fresh listener on the same port number.
func redialer(tport transport.Transport, port uint32) func() (transport.Connection, error) {
	return func() (transport.Connection, error) {
		nc, err := tport.Dial(port)
		if err != nil {
			return nil, err
		}
		return transport.NewLogConnection(nc, port), nil
	}
}
