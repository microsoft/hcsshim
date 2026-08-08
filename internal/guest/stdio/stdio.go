//go:build linux
// +build linux

package stdio

import (
	"os"
	"strings"
	"sync"
	"time"

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
	// Subscribed here, not at the first round, so a bridge reconnect between
	// construction and the first round still invalidates these connections.
	pr := &PipeRelay{s: s, bridge: newBridgeWatcher()}
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
	// closing makes setConn re-apply Wait's stdin CloseRead when Wait overlaps a
	// redial; without it that one-shot lands on the discarded conn and Wait hangs.
	// It deliberately does not cancel the redial.
	closing bool
	// bridge is this relay's durable bridge-reconnect subscription. Owned by the
	// run goroutine, so it needs no lock.
	bridge *bridgeWatcher
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
	pr.done = make(chan struct{})
	go pr.run()
}

// setConn publishes ns as the current connection set (nil when tearing down)
// and closes the previous set. It holds mu across the swap and close so the
// close cannot race Wait's CloseRead on the same connection; the slow redial
// stays off the lock in the caller.
func (pr *PipeRelay) setConn(ns *ConnectionSet) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if pr.s != nil {
		pr.s.Close()
	}
	pr.s = ns
	if ns != nil && ns.In != nil && pr.closing {
		_ = ns.In.CloseRead()
	}
}

// run restarts the copiers after a connection interruption and tears down the
// pipes and connections once copying finishes.
func (pr *PipeRelay) run() {
	// Pending output is replayed after redial.
	var outPending, errPending []byte
	for {
		var err error
		outPending, errPending, err = pr.runCopiers(outPending, errPending)
		if !errors.Is(err, errNeedsRedial) {
			break
		}
		// run is the only writer of pr.s, so it reads the redial closure without
		// mu; the dial can block for seconds and must stay off the lock.
		ns, derr := redialWithRetry(pr.s.redial)
		if derr != nil {
			logrus.WithError(derr).Error("opengcs::PipeRelay::run - redial failed; ending relay")
			break
		}
		pr.setConn(ns)
	}
	pr.closePipes()
	pr.setConn(nil)
	close(pr.done)
}

// runCopiers starts one copier per stream and waits for all of them. It reports
// errNeedsRedial if a host connection failed and must be redialed, plus output
// to replay.
func (pr *PipeRelay) runCopiers(outPending, errPending []byte) (outPend, errPend []byte, err error) {
	// run is the only writer of pr.s and does not swap it until this returns, so
	// reading it here without mu is safe.
	s := pr.s
	stdin := s.In
	stdout := s.Out
	stderr := s.Err
	canRedial := s.redial != nil

	// Capture the reader files up front, like s above: a copier must not read
	// pr.pipes[i] while the stdin copier clears pr.pipes[1]. CloseUnusedPipes
	// closes unused read ends without nilling the fields, so only take the ones
	// that actually have a copier.
	var readers []*os.File
	if stdout != nil {
		readers = append(readers, pr.pipes[2])
	}
	if stderr != nil {
		readers = append(readers, pr.pipes[4])
	}

	// A deadline left armed by a previous round's wake would fail every read
	// instantly, which reads as errNeedsRedial and spins the manager.
	if derr := setReadDeadlines(readers, time.Time{}); derr != nil {
		logrus.WithError(derr).Error("opengcs::PipeRelay::runCopiers - cannot restore blocking reads; ending relay")
		return outPending, errPending, derr
	}

	sig := newRedialSignal()
	if canRedial {
		// Only when a redial is possible: a forced wake on a set without a redial
		// closure would make run call a nil func.
		if pr.bridge.refresh() {
			// Reconnected while this relay was between rounds, so these
			// connections are already dead.
			sig.raise()
		}
		defer watchForRedial(sig, pr.bridge, stdin, readers)()
	}

	var cwg sync.WaitGroup
	// Each result has one writer and is read after cwg.Wait.
	var stdinErr, stdoutErr, stderrErr error

	if stdin != nil && pr.pipes[1] != nil {
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			stdinErr = copyIn(pr.pipes[1], stdin, "stdin", canRedial, sig)
			if errors.Is(stdinErr, errNeedsRedial) {
				sig.raise()
				// Leave stdin open across redial.
				return
			}
			// Close stdin on a normal end so the process observes EOF.
			if cerr := pr.pipes[1].Close(); cerr != nil {
				logrus.WithError(cerr).Error("opengcs::PipeRelay::runCopiers - error closing stdin write pipe")
			}
			pr.pipes[1] = nil
		}()
	}
	if stdout != nil {
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			outPend, stdoutErr = copyOut(stdout, pr.pipes[2], "stdout", outPending, canRedial)
			if errors.Is(stdoutErr, errNeedsRedial) {
				sig.raise()
			}
		}()
	}
	if stderr != nil {
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			errPend, stderrErr = copyOut(stderr, pr.pipes[4], "stderr", errPending, canRedial)
			if errors.Is(stderrErr, errNeedsRedial) {
				sig.raise()
			}
		}()
	}
	cwg.Wait()
	if errors.Is(stdinErr, errNeedsRedial) || errors.Is(stdoutErr, errNeedsRedial) || errors.Is(stderrErr, errNeedsRedial) {
		err = errNeedsRedial
	}
	return outPend, errPend, err
}

// Wait waits for the relaying to finish and closes the associated
// pipes and connections.
func (pr *PipeRelay) Wait() {
	// Close stdin's read side and snapshot done under the lock so the CloseRead
	// cannot race the relay manager goroutine's Close of the same ConnectionSet
	// (the redial swap or the final teardown).
	pr.mu.Lock()
	// Recorded under mu so a redial that swaps in a fresh set after this point
	// re-applies the CloseRead below; see setConn.
	pr.closing = true
	// Close stdin so that the copying goroutine is safely unblocked; this is necessary
	// because the host expects stdin to be closed before it will report process
	// exit back to the client, and the client expects the process notification before
	// it will close its side of stdin (which the input copier is blocked reading).
	if pr.s != nil && pr.s.In != nil {
		_ = pr.s.In.CloseRead()
	}
	done := pr.done
	pr.mu.Unlock()

	if done == nil {
		// Start was never called; tear down synchronously.
		pr.closePipes()
		pr.setConn(nil)
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
	return &TtyRelay{s: s, pty: pty, bridge: newBridgeWatcher()}
}

// TtyRelay relays IO between a set of stdio connections and a master PTY file.
type TtyRelay struct {
	// m guards closed, closing, the pty teardown, and s (which the relay manager
	// goroutine swaps when it re-dials the stdio connections after a bridge drop).
	m      sync.Mutex
	closed bool
	// closing records that Wait has been called; see PipeRelay.closing.
	closing bool
	s       *ConnectionSet
	pty     *os.File
	// bridge is this relay's durable bridge-reconnect subscription. Owned by the
	// run goroutine, so it needs no lock.
	bridge *bridgeWatcher
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
	r.done = make(chan struct{})
	go r.run()
}

// setConn publishes ns as the current connection set and closes the previous
// set under m so the close cannot race Wait's CloseRead on the same connection.
func (r *TtyRelay) setConn(ns *ConnectionSet) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.s != nil {
		r.s.Close()
	}
	r.s = ns
	if ns != nil && ns.In != nil && r.closing {
		_ = ns.In.CloseRead()
	}
}

// teardown closes the pty and the current connection set under m, which also
// guards ResizeConsole's use of the pty.
func (r *TtyRelay) teardown() {
	r.m.Lock()
	defer r.m.Unlock()
	if r.closed {
		return
	}
	r.pty.Close()
	r.closed = true
	if r.s != nil {
		r.s.Close()
		r.s = nil
	}
}

// run restarts the TTY copiers after a connection interruption and tears down
// the pty and connections once copying finishes.
func (r *TtyRelay) run() {
	// Pending output is replayed after redial.
	var outPending []byte
	for {
		var err error
		outPending, err = r.runCopiers(outPending)
		if !errors.Is(err, errNeedsRedial) {
			break
		}
		// run is the only writer of r.s, so it reads the redial closure without
		// m; the dial can block for seconds and must stay off the lock.
		ns, derr := redialWithRetry(r.s.redial)
		if derr != nil {
			logrus.WithError(derr).Error("opengcs::TtyRelay::run - redial failed; ending relay")
			break
		}
		r.setConn(ns)
	}
	r.teardown()
	close(r.done)
}

// runCopiers starts the stdin and stdout copiers and waits for both. It reports
// errNeedsRedial if a host connection failed and must be redialed, plus output
// to replay.
func (r *TtyRelay) runCopiers(outPending []byte) (outPend []byte, err error) {
	// run is the only writer of r.s and does not swap it until this returns, so
	// reading it here without m is safe.
	s := r.s
	stdin := s.In
	stdout := s.Out
	canRedial := s.redial != nil

	// The pty master is both the stdout read source and the stdin write sink, so
	// only its READ deadline may be armed; SetDeadline would also fail input.
	var readers []*os.File
	if stdout != nil {
		readers = append(readers, r.pty)
	}
	if derr := setReadDeadlines(readers, time.Time{}); derr != nil {
		logrus.WithError(derr).Error("opengcs::TtyRelay::runCopiers - cannot restore blocking reads; ending relay")
		return outPending, derr
	}

	sig := newRedialSignal()
	if canRedial {
		if r.bridge.refresh() {
			sig.raise()
		}
		defer watchForRedial(sig, r.bridge, stdin, readers)()
	}

	var cwg sync.WaitGroup
	// Each result has one writer and is read after cwg.Wait.
	var stdinErr, stdoutErr error

	if stdin != nil {
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			stdinErr = copyIn(r.pty, stdin, "stdin", canRedial, sig)
			if errors.Is(stdinErr, errNeedsRedial) {
				sig.raise()
			}
		}()
	}
	if stdout != nil {
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			outPend, stdoutErr = copyOut(stdout, r.pty, "stdout", outPending, canRedial)
			if errors.Is(stdoutErr, errNeedsRedial) {
				sig.raise()
			}
		}()
	}
	cwg.Wait()
	if errors.Is(stdinErr, errNeedsRedial) || errors.Is(stdoutErr, errNeedsRedial) {
		err = errNeedsRedial
	}
	return outPend, err
}

// Wait waits for the relaying to finish and closes the associated
// files and connections.
func (r *TtyRelay) Wait() {
	// Close stdin's read side and snapshot done under the lock so the CloseRead
	// cannot race the relay manager goroutine's Close of the same ConnectionSet
	// (the redial swap or the final teardown).
	r.m.Lock()
	// Recorded under m so a redial that swaps in a fresh set after this point
	// re-applies the CloseRead below; see setConn.
	r.closing = true
	// Close stdin so that the copying goroutine is safely unblocked; this is necessary
	// because the host expects stdin to be closed before it will report process
	// exit back to the client, and the client expects the process notification before
	// it will close its side of stdin (which the input copier is blocked reading).
	if r.s != nil && r.s.In != nil {
		_ = r.s.In.CloseRead()
	}
	done := r.done
	r.m.Unlock()

	if done == nil {
		// Start was never called; tear down synchronously.
		r.teardown()
		return
	}

	<-done
}
