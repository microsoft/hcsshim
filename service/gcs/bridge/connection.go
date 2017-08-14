package bridge

import (
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/pkg/errors"
)

// connectStdio returns new transport.Connection instances, one for each
// stdio pipe to be used. If CreateStd*Pipe for a given pipe is false, the
// given Connection is set to nil.
func connectStdio(tport transport.Transport, params prot.ProcessParameters, settings prot.ExecuteProcessVsockStdioRelaySettings) (_ *stdio.ConnectionSet, err error) {
	connSet := &stdio.ConnectionSet{}
	defer func() {
		if err != nil {
			connSet.Close()
		}
	}()
	if params.CreateStdInPipe {
		connSet.In, err = tport.Dial(settings.StdIn)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdin Connection")
		}
	}
	if params.CreateStdOutPipe {
		connSet.Out, err = tport.Dial(settings.StdOut)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
	}
	if params.CreateStdErrPipe {
		connSet.Err, err = tport.Dial(settings.StdErr)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
	}
	return connSet, nil
}
