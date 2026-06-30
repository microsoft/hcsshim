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
// is set to nil. The returned set carries a redial closure so the stdio relays
// can re-establish the connections over the same vsock ports and pause and
// resume the process stdio across a live-migration bridge drop.
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
		connSet.In = transport.NewLogConnection(c, port)
	}
	if settings.StdOut != nil {
		port := *settings.StdOut
		c, err := tport.Dial(port)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
		connSet.Out = transport.NewLogConnection(c, port)
	}
	if settings.StdErr != nil {
		port := *settings.StdErr
		c, err := tport.Dial(port)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
		connSet.Err = transport.NewLogConnection(c, port)
	}
	// redial re-establishes a fresh ConnectionSet over the same vsock ports
	// after a bridge drop, so the stdio relays can pause and resume across a
	// live migration instead of tearing the process stdio down.
	connSet.redial = func() (*ConnectionSet, error) {
		return Connect(tport, settings)
	}
	return connSet, nil
}
