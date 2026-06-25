//go:build linux
// +build linux

package stdio

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ConnectionSet is a structure defining the readers and writers the Core
// implementation should forward a process's stdio through.
type ConnectionSet struct {
	In, Out, Err transport.Connection
	// redial, when non-nil, re-establishes a fresh ConnectionSet over the same
	// vsock ports. The relay manager goroutine uses it to recover the process
	// stdio after a live-migration bridge drop.
	redial func() (*ConnectionSet, error)
}

// Close closes each stdio connection.
func (s *ConnectionSet) Close() error {
	var err error
	if s.In != nil {
		if cerr := s.In.Close(); cerr != nil {
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

// FileSet represents the stdio of a process. It contains os.File types for
// in, out, err.
type FileSet struct {
	In, Out, Err *os.File
}

// Close closes all the FileSet handles.
func (fs *FileSet) Close() error {
	var err error
	if fs.In != nil {
		if cerr := fs.In.Close(); cerr != nil {
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

// NewPipeRelay returns a new pipe relay wrapping the given connection stdin,
// stdout, stderr set. If s is nil will assume al stdin, stdout, stderr pipes.
func NewPipeRelay(s *ConnectionSet) (_ *PipeRelay, err error) {
	pr := &PipeRelay{s: s}
	defer func() {
		if err != nil {
			pr.closePipes()
		}
	}()

	if s == nil || s.In != nil {
		pr.pipes[0], pr.pipes[1], err = os.Pipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stdin pipe relay")
		}
	}
	if s == nil || s.Out != nil {
		pr.pipes[2], pr.pipes[3], err = os.Pipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stdout pipe relay")
		}
	}
	if s == nil || s.Err != nil {
		pr.pipes[4], pr.pipes[5], err = os.Pipe()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create stderr pipe relay")
		}
	}
	return pr, nil
}

// PipeRelay is a relay built to expose a pipe interface
// for stdin, stdout, stderr on top of a ConnectionSet.
type PipeRelay struct {
	// mu guards s, which the relay manager goroutine swaps when it re-dials the
	// stdio connections after a bridge drop.
	mu sync.Mutex
	s  *ConnectionSet
	// pipes format is stdin [0 read, 1 write], stdout [2 read, 3 write], stderr [4 read, 5 write].
	pipes [6]*os.File
	// pauseOnError is true when s carries a redial closure, meaning a copy error
	// caused by a bridge drop should pause and resume rather than tear down the
	// process stdio.
	pauseOnError bool
	// done is closed by the relay manager goroutine once the relay is truly
	// finished (process exit), so Wait can block until then across any pauses.
	done chan struct{}
}

// ReplaceConnectionSet allows the caller to add a new destination set after
// creating the relay. This can only be called previous to the call to Start.
func (pr *PipeRelay) ReplaceConnectionSet(s *ConnectionSet) {
	pr.s = s
}

// Files returns a FileSet with an os.File for each connection
// in the connection set.
func (pr *PipeRelay) Files() (*FileSet, error) {
	fs := new(FileSet)
	if pr.s == nil || pr.s.In != nil {
		fs.In = pr.pipes[0]
	}
	if pr.s == nil || pr.s.Out != nil {
		fs.Out = pr.pipes[3]
	}
	if pr.s == nil || pr.s.Err != nil {
		fs.Err = pr.pipes[5]
	}
	return fs, nil
}

// Start starts the relay operation. The caller must call Wait to wait
// for the relay to finish and release the associated resources.
func (pr *PipeRelay) Start() {
	pr.pauseOnError = pr.s != nil && pr.s.redial != nil
	pr.done = make(chan struct{})
	go pr.run()
}

// run manages the relay copiers across live-migration pauses. It runs the
// copiers, and if they exit because the bridge dropped, re-dials the stdio
// connections and restarts the copiers so the process stdio survives the
// migration. When the copiers finish for a non-pause reason (process exit) it
// tears down the pipes and connections and signals done.
func (pr *PipeRelay) run() {
	// outPending and errPending carry the in-flight bytes a copier read from
	// the process pipe but had not yet written to the host conn when the bridge
	// dropped, so the next iteration replays them on the re-dialed conn.
	var outPending, errPending []byte
	for {
		var paused bool
		paused, outPending, errPending = pr.runCopiers(outPending, errPending)
		if !paused {
			break
		}
		pr.mu.Lock()
		redial := pr.s.redial
		pr.mu.Unlock()
		ns, err := redialWithRetry(redial)
		if err != nil {
			logrus.WithError(err).Error("opengcs::PipeRelay::run - redial failed; ending relay")
			break
		}
		// Swap in the re-dialed set and close the old one all under the lock so
		// the conn Close cannot race Wait's CloseRead on the same conn. The dial
		// above stays outside the lock because it can block for seconds.
		pr.mu.Lock()
		old := pr.s
		pr.s = ns
		old.Close()
		pr.mu.Unlock()
	}
	pr.closePipes()
	// Close the live set under the lock so the conn Close cannot race Wait's
	// CloseRead; closePipes stays outside since it only touches the pipes.
	pr.mu.Lock()
	s := pr.s
	pr.s = nil
	if s != nil {
		s.Close()
	}
	pr.mu.Unlock()
	close(pr.done)
}

// runCopiers launches one copier per present stdio stream and waits for them
// all to finish. outPending/errPending are the in-flight output remainders held
// from a prior bridge-drop pause; each is replayed on the (re-dialed) conn
// before fresh output. It returns true if any copier paused because of a bridge
// drop (a live migration), along with the new held remainders for stdout and
// stderr so the caller can thread them into the next iteration.
func (pr *PipeRelay) runCopiers(outPending, errPending []byte) (paused bool, outPend, errPend []byte) {
	pr.mu.Lock()
	s := pr.s
	pr.mu.Unlock()

	var cwg sync.WaitGroup
	var pausedFlag atomic.Bool

	if s.In != nil && pr.pipes[1] != nil {
		in := s.In
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			if copyIn(pr.pipes[1], in, "stdin", pr.pauseOnError) {
				// Live migration pause: leave the stdin write pipe open so the
				// process keeps its stdin across the redial.
				pausedFlag.Store(true)
				return
			}
			// Normal end (host closed stdin, or the process closed its stdin):
			// close the stdin write pipe so the process observes EOF, matching
			// the original relay behavior.
			if cerr := pr.pipes[1].Close(); cerr != nil {
				logrus.WithError(cerr).Error("opengcs::PipeRelay::runCopiers - error closing stdin write pipe")
			}
			pr.pipes[1] = nil
		}()
	}
	if s.Out != nil {
		out := s.Out
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			held, p := copyOut(out, pr.pipes[2], "stdout", outPending, pr.pauseOnError)
			outPend = held
			if p {
				pausedFlag.Store(true)
			}
		}()
	}
	if s.Err != nil {
		errc := s.Err
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			held, p := copyOut(errc, pr.pipes[4], "stderr", errPending, pr.pauseOnError)
			errPend = held
			if p {
				pausedFlag.Store(true)
			}
		}()
	}
	cwg.Wait()
	return pausedFlag.Load(), outPend, errPend
}

// Wait waits for the relaying to finish and closes the associated
// pipes and connections.
func (pr *PipeRelay) Wait() {
	// Snapshot the set, close stdin's read side, and snapshot done all under the
	// lock so the CloseRead cannot race the relay manager goroutine's Close of
	// the same ConnectionSet (the redial swap or the final teardown).
	pr.mu.Lock()
	s := pr.s
	// Close stdin so that the copying goroutine is safely unblocked; this is necessary
	// because the host expects stdin to be closed before it will report process
	// exit back to the client, and the client expects the process notification before
	// it will close its side of stdin (which the input copier is blocked reading).
	if s != nil && s.In != nil {
		_ = s.In.CloseRead()
	}
	done := pr.done
	pr.mu.Unlock()

	if done == nil {
		// Start was never called (no stdio requested); tear down synchronously
		// to match the original no-op relay behavior. Close the set under the
		// lock for consistency with the relay manager goroutine's teardown.
		pr.closePipes()
		pr.mu.Lock()
		s = pr.s
		pr.s = nil
		if s != nil {
			s.Close()
		}
		pr.mu.Unlock()
		return
	}

	<-done
}

// CloseUnusedPipes gives the caller the ability to close any pipes that do not
// have a corresponding entry on the ConnectionSet. This is to be used in
// conjunction with NewPipeRelay where s is nil which wil open all pipes and
// later calling ReplaceConnectionSet with the actual connections.
func (pr *PipeRelay) CloseUnusedPipes() {
	if pr.s == nil {
		pr.closePipes()
	} else {
		if pr.s.In == nil {
			// Write end of stdin
			pr.pipes[1].Close()
		}
		if pr.s.Out == nil {
			// Read end of stdout
			pr.pipes[2].Close()
		}
		if pr.s.Err == nil {
			// Read end of stderr
			pr.pipes[4].Close()
		}
	}
}

func (pr *PipeRelay) closePipes() {
	for i := 0; i < len(pr.pipes); i++ {
		if pr.pipes[i] != nil {
			if err := pr.pipes[i].Close(); err != nil {
				if !strings.Contains(err.Error(), "file already closed") {
					logrus.WithFields(logrus.Fields{
						logrus.ErrorKey: err,
					}).Error("opengcs::PipeRelay::closePipes - error closing relay pipe")
				}
			}
			pr.pipes[i] = nil
		}
	}
}

// NewTtyRelay returns a new TTY relay for a given master PTY file.
func NewTtyRelay(s *ConnectionSet, pty *os.File) *TtyRelay {
	return &TtyRelay{s: s, pty: pty}
}

// TtyRelay relays IO between a set of stdio connections and a master PTY file.
type TtyRelay struct {
	// m guards closed, the pty teardown, and s (which the relay manager goroutine
	// swaps when it re-dials the stdio connections after a bridge drop).
	m      sync.Mutex
	closed bool
	s      *ConnectionSet
	pty    *os.File
	// pauseOnError is true when s carries a redial closure, meaning a copy error
	// caused by a bridge drop should pause and resume.
	pauseOnError bool
	// done is closed once the relay is truly finished, so Wait can block across
	// any live-migration pauses.
	done chan struct{}
}

// ReplaceConnectionSet allows the caller to add a new destination set after
// creating the relay. This can only be called previous to the call to Start.
func (r *TtyRelay) ReplaceConnectionSet(s *ConnectionSet) {
	r.s = s
}

// ResizeConsole sends the appropriate resize to a pTTY FD.
func (r *TtyRelay) ResizeConsole(height, width uint16) error {
	r.m.Lock()
	defer r.m.Unlock()

	if r.closed {
		return nil
	}
	return ResizeConsole(r.pty, height, width)
}

// Start starts the relay operation. The caller must call Wait to wait
// for the relay to finish and release the associated resources.
func (r *TtyRelay) Start() {
	r.pauseOnError = r.s != nil && r.s.redial != nil
	r.done = make(chan struct{})
	go r.run()
}

// run manages the TTY relay copiers across live-migration pauses, re-dialing
// and restarting them when the bridge drops, and closing the pty and
// connections once the copiers finish for a non-pause reason (process exit).
func (r *TtyRelay) run() {
	// outPending carries the in-flight pty output bytes read but not yet
	// written to the host conn when the bridge dropped, replayed next iteration.
	var outPending []byte
	for {
		var paused bool
		paused, outPending = r.runCopiers(outPending)
		if !paused {
			break
		}
		r.m.Lock()
		redial := r.s.redial
		r.m.Unlock()
		ns, err := redialWithRetry(redial)
		if err != nil {
			logrus.WithError(err).Error("opengcs::TtyRelay::run - redial failed; ending relay")
			break
		}
		// Swap in the re-dialed set and close the old one all under the lock so
		// the conn Close cannot race Wait's CloseRead on the same conn. The dial
		// above stays outside the lock because it can block for seconds.
		r.m.Lock()
		old := r.s
		r.s = ns
		old.Close()
		r.m.Unlock()
	}
	// Close the pty and the live set under the lock so the conn Close cannot race
	// Wait's CloseRead.
	r.m.Lock()
	r.pty.Close()
	r.closed = true
	s := r.s
	r.s = nil
	if s != nil {
		s.Close()
	}
	r.m.Unlock()
	close(r.done)
}

// runCopiers launches the stdin->pty and pty->stdout copiers and waits for them
// to finish. outPending is the in-flight pty output remainder held from a prior
// bridge-drop pause, replayed on the (re-dialed) conn before fresh output. It
// returns true if a copier paused because the bridge dropped during a live
// migration, along with the new held output remainder for the next iteration.
func (r *TtyRelay) runCopiers(outPending []byte) (paused bool, outPend []byte) {
	r.m.Lock()
	s := r.s
	r.m.Unlock()

	var cwg sync.WaitGroup
	var pausedFlag atomic.Bool

	if s.In != nil {
		in := s.In
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			if copyIn(r.pty, in, "stdin", r.pauseOnError) {
				pausedFlag.Store(true)
			}
		}()
	}
	if s.Out != nil {
		out := s.Out
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			held, p := copyOut(out, r.pty, "stdout", outPending, r.pauseOnError)
			outPend = held
			if p {
				pausedFlag.Store(true)
			}
		}()
	}
	cwg.Wait()
	return pausedFlag.Load(), outPend
}

// Wait waits for the relaying to finish and closes the associated
// files and connections.
func (r *TtyRelay) Wait() {
	// Snapshot the set, close stdin's read side, and snapshot done all under the
	// lock so the CloseRead cannot race the relay manager goroutine's Close of
	// the same ConnectionSet (the redial swap or the final teardown).
	r.m.Lock()
	s := r.s
	// Close stdin so that the copying goroutine is safely unblocked; this is necessary
	// because the host expects stdin to be closed before it will report process
	// exit back to the client, and the client expects the process notification before
	// it will close its side of stdin (which the input copier is blocked reading).
	if s != nil && s.In != nil {
		_ = s.In.CloseRead()
	}
	done := r.done
	r.m.Unlock()

	if done == nil {
		// Start was never called; tear down synchronously to match the original
		// no-op relay behavior. Close the set under the lock for consistency with
		// the relay manager goroutine's teardown.
		r.m.Lock()
		r.pty.Close()
		r.closed = true
		s = r.s
		r.s = nil
		if s != nil {
			s.Close()
		}
		r.m.Unlock()
		return
	}

	<-done
}
