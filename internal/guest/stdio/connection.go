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
// is set to nil.
func Connect(tport transport.Transport, settings ConnectionSettings) (_ *ConnectionSet, err error) {
	connSet := &ConnectionSet{}
	defer func() {
		if err != nil {
			connSet.Close()
		}
	}()
	if settings.StdIn != nil {
		c, err := tport.Dial(*settings.StdIn)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdin Connection")
		}
		connSet.In = transport.NewLogConnection(c, *settings.StdIn)
	}
	if settings.StdOut != nil {
		c, err := tport.Dial(*settings.StdOut)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
		connSet.Out = transport.NewLogConnection(c, *settings.StdOut)
	}
	if settings.StdErr != nil {
		c, err := tport.Dial(*settings.StdErr)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
		connSet.Err = transport.NewLogConnection(c, *settings.StdErr)
	}
	return connSet, nil
}
