package stdio

import (
	"io"
	"os"
	"sync"

	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/sirupsen/logrus"
	"github.com/pkg/errors"
)

// ConnectionSet is a structure defining the readers and writers the Core
// implementation should forward a process's stdio through.
type ConnectionSet struct {
	In, Out, Err transport.Connection
}

// Close closes each stdio connection.
func (s *ConnectionSet) Close() error {
	var err error
	if s.In != nil {
		if cerr := s.In.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "failed Close on stdin")
		}
		s.In = nil
	}
	if s.Out != nil {
		if cerr := s.Out.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "failed Close on stdout")
		}
		s.Out = nil
	}
	if s.Err != nil {
		if cerr := s.Err.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "failed Close on stderr")
		}
		s.Err = nil
	}
	return err
}

// FileSet contains os.File fields for stdio.
type FileSet struct {
	In, Out, Err *os.File
}

// Close closes all the FileSet handles.
func (fs *FileSet) Close() error {
	var err error
	if fs.In != nil {
		if cerr := fs.In.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "failed Close on stdin")
		}
		fs.In = nil
	}
	if fs.Out != nil {
		if cerr := fs.Out.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "failed Close on stdout")
		}
		fs.Out = nil
	}
	if fs.Err != nil {
		if cerr := fs.Err.Close(); cerr != nil && err == nil {
			err = errors.Wrap(cerr, "failed Close on stderr")
		}
		fs.Err = nil
	}
	return err
}

// Files returns a FileSet with an os.File for each connection
// in the connection set.
func (s *ConnectionSet) Files() (_ *FileSet, err error) {
	fs := &FileSet{}
	defer func() {
		if err != nil {
			fs.Close()
		}
	}()
	if s.In != nil {
		fs.In, err = s.In.File()
		if err != nil {
			return nil, errors.Wrap(err, "failed to dup stdin socket for command")
		}
	}
	if s.Out != nil {
		fs.Out, err = s.Out.File()
		if err != nil {
			return nil, errors.Wrap(err, "failed to dup stdout socket for command")
		}
	}
	if s.Err != nil {
		fs.Err, err = s.Err.File()
		if err != nil {
			return nil, errors.Wrap(err, "failed to dup stderr socket for command")
		}
	}
	return fs, nil
}

// NewTtyRelay returns a new TTY relay for a given master PTY file.
func (s *ConnectionSet) NewTtyRelay(pty io.ReadWriteCloser) *TtyRelay {
	return &TtyRelay{s: s, pty: pty}
}

// TtyRelay relays IO between a set of stdio connections and a master PTY file.
type TtyRelay struct {
	wg  sync.WaitGroup
	s   *ConnectionSet
	pty io.ReadWriteCloser
}

// Start starts the relay operation. The caller must call Wait to wait
// for the relay to finish and release the associated resources.
func (r *TtyRelay) Start() {
	if r.s.In != nil {
		r.wg.Add(1)
		go func() {
			_, err := io.Copy(r.pty, r.s.In)
			if err != nil {
				logrus.Errorf("error copying stdin to pty: %s", err)
			}
			r.wg.Done()
		}()
	}
	if r.s.Out != nil {
		r.wg.Add(1)
		go func() {
			_, err := io.Copy(r.s.Out, r.pty)
			if err != nil {
				logrus.Errorf("error copying pty to stdout: %s", err)
			}
			r.wg.Done()
		}()
	}
}

// Wait waits for the relaying to finish and closes the associated
// files and connections.
func (r *TtyRelay) Wait() {
	// Close stdin so that the copying goroutine is safely unblocked; this is necessary
	// because the host expects stdin to be closed before it will report process
	// exit back to the client, and the client expects the process notification before
	// it will close its side of stdin (which io.Copy is waiting on in the copying goroutine).
	if r.s.In != nil {
		if err := r.s.In.CloseRead(); err != nil {
			logrus.Errorf("error closing read for stdin: %s", err)
		}
	}

	// Wait for all users of stdioSet and master to finish before closing them.
	r.wg.Wait()
	r.pty.Close()
	r.s.Close()
}
