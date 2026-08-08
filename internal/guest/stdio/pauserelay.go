//go:build linux
// +build linux

package stdio

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// redialInterval is the delay between attempts to re-establish the stdio
	// connections after a bridge drop.
	redialInterval = 100 * time.Millisecond
	// redialTimeout bounds how long a relay waits for the bridge to come back
	// before giving up and tearing the process stdio down. It must cover a
	// live-migration resume: the destination can take well over a few seconds to
	// resume the UVM and re-establish the stdio bridge, so a short bound risks
	// giving up before the destination is ready, which stops draining the process
	// stdout pipe and blocks the process once the pipe fills.
	//
	// A wall-clock bound rather than an attempt count because each attempt is a
	// blocking dial of up to three vsock ports; counting attempts would stretch
	// this to minutes whenever the dials are slow to fail.
	//
	// Sized against what one failed attempt costs, not against how long a healthy
	// resume takes. VsockTransport.Dial retries ETIMEDOUT ten times internally, so
	// a Connect to a port nothing is listening on takes roughly 20s to fail; a 60s
	// budget therefore bought only about three tries before severing the process's
	// stdio for good. Giving up is the expensive direction: a relay that keeps
	// retrying costs one goroutine, while one that stops leaves the container
	// wedged on a full stdout pipe with no way back.
	redialTimeout = 5 * time.Minute
)

// redialWithRetry re-establishes a ConnectionSet using the provided redial
// closure, retrying every redialInterval until redialTimeout elapses. It returns
// the new set on success or the last error once the budget is spent.
func redialWithRetry(redial func() (*ConnectionSet, error)) (*ConnectionSet, error) {
	return redialWithBudget(redial, redialTimeout)
}

func redialWithBudget(redial func() (*ConnectionSet, error), budget time.Duration) (*ConnectionSet, error) {
	// time.Now carries a monotonic reading, so this deadline is unaffected by the
	// wall-clock correction a just-migrated guest takes.
	deadline := time.Now().Add(budget)
	for attempt := 1; ; attempt++ {
		ns, err := redial()
		if err == nil {
			if attempt > 1 {
				logrus.WithField("attempts", attempt).Info("opengcs::stdio::redial - stdio reconnected")
			}
			return ns, nil
		}
		if !time.Now().Before(deadline) {
			return nil, err
		}
		// Logged per attempt because the host silently not listening on a stdio
		// port is otherwise only visible by capturing shim ETW.
		logrus.WithError(err).WithField("attempt", attempt).
			Warn("opengcs::stdio::redial - could not re-establish stdio; retrying")
		time.Sleep(redialInterval)
	}
}

// bridgeCh is closed and replaced each time the guest's GCS bridge re-dials the
// host, and bridgeGen counts those events. One bridge per GCS, so a drop is
// process-wide and the signal is package scoped rather than threaded through
// Host and Container to every relay.
//
// The counter is what makes the signal durable. A closed channel is edge
// triggered: an event that lands while a relay is between rounds, or inside
// redialWithRetry, would be dropped and the relay would keep copying into dead
// connections forever.
var (
	bridgeMu  sync.Mutex
	bridgeGen uint64
	bridgeCh  = make(chan struct{})
)

// NotifyBridgeReconnected reports that the GCS bridge reconnected, which is what
// happens on the destination of a live migration. Relays treat it as "your stdio
// connections are stale".
//
// This is the authoritative trigger; a copier's write failure is secondary,
// since it cannot fire for a stream with no traffic.
func NotifyBridgeReconnected() {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	bridgeGen++
	close(bridgeCh)
	bridgeCh = make(chan struct{})
}

func bridgeSignal() (uint64, <-chan struct{}) {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	return bridgeGen, bridgeCh
}

// bridgeWatcher is one relay's subscription, held across rounds so no event is
// missed between them. It is owned by the relay's run goroutine; the only other
// accessor is the watcher goroutine, which watchForRedial's stop joins.
type bridgeWatcher struct {
	gen uint64
	ch  <-chan struct{}
}

func newBridgeWatcher() *bridgeWatcher {
	w := &bridgeWatcher{}
	w.gen, w.ch = bridgeSignal()
	return w
}

// refresh re-subscribes for the coming round and reports whether the bridge
// reconnected since the last look, which means the round starts already stale.
func (w *bridgeWatcher) refresh() bool {
	gen, ch := bridgeSignal()
	w.ch = ch
	if gen == w.gen {
		return false
	}
	w.gen = gen
	return true
}

// consume records the event the watcher goroutine just observed, so the next
// round's refresh does not report it a second time and redial twice.
func (w *bridgeWatcher) consume() { w.gen, _ = bridgeSignal() }

// redialSignal is a close-once notification scoped to one copier round. It also
// lets copyIn recognise a forced EOF.
type redialSignal struct {
	once sync.Once
	c    chan struct{}
}

func newRedialSignal() *redialSignal { return &redialSignal{c: make(chan struct{})} }

func (s *redialSignal) raise() { s.once.Do(func() { close(s.c) }) }

func (s *redialSignal) raised() bool {
	select {
	case <-s.c:
		return true
	default:
		return false
	}
}

// pastDeadline is already elapsed, so arming it cancels an in-flight read now
// rather than scheduling a timeout. A fixed value is immune to the clock jumps a
// just-migrated guest sees.
var pastDeadline = time.Unix(1, 0)

// setReadDeadlines applies t to every relay-owned reader: pastDeadline to cancel
// an in-flight read, the zero time to restore blocking reads.
//
// os.ErrNoDeadline means the reader is not pollable, so that stream cannot be
// interrupted and the round waits for it to end on its own.
func setReadDeadlines(readers []*os.File, t time.Time) error {
	for _, f := range readers {
		if f == nil {
			continue
		}
		if err := f.SetReadDeadline(t); err != nil && !errors.Is(err, os.ErrNoDeadline) {
			return errors.Wrapf(err, "setting read deadline on %s", f.Name())
		}
	}
	return nil
}

// wakeParkedCopiers interrupts pipe/pty reads with deadlines and the blocking
// vsock stdin read with CloseRead. The container's own pipe ends stay open, so
// it keeps its stdio across the redial.
//
// CloseRead rather than Close: a deadline is useless on the host conn (the
// vendored vsockConn.SetReadDeadline is a no-op stub and the fd is a blocking
// socket Go never polls) and Close does not interrupt an in-flight read, only
// shutdown(2) does. CloseRead also leaves the descriptor allocated, so a
// concurrent Wait cannot shut down a recycled one.
func wakeParkedCopiers(stdin transport.Connection, readers []*os.File) {
	if err := setReadDeadlines(readers, pastDeadline); err != nil {
		logrus.WithError(err).Error("opengcs::stdio::wakeParkedCopiers - failed to interrupt a parked reader")
	}
	if stdin != nil {
		if err := stdin.CloseRead(); err != nil {
			logrus.WithError(err).Debug("opengcs::stdio::wakeParkedCopiers - CloseRead on the dropped stdin conn")
		}
	}
}

// watchForRedial ends the round when the bridge reconnects or a copier raises
// sig, then wakes the parked copiers. Callers must only use it when the set can
// actually redial; a forced wake on a set without a redial closure would make
// run call a nil func.
//
// stop joins the watcher, because a read deadline is sticky fd state and a
// watcher still running after the round could arm one the next round has
// already cleared. The join is also what makes w safe to touch here.
func watchForRedial(sig *redialSignal, w *bridgeWatcher, stdin transport.Connection, readers []*os.File) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		select {
		case <-w.ch:
			w.consume()
			// Raised before the wake so a woken copyIn can tell it from a real EOF.
			sig.raise()
		case <-sig.c:
		case <-done:
			return
		}
		wakeParkedCopiers(stdin, readers)
	}()
	return func() {
		close(done)
		<-finished
	}
}

// relayBufferSize is the size of the per-stream copy buffer the output relays
// read into before writing to the host connection. It matches io.Copy's
// default buffer size; the unwritten tail of one of these buffers is what is
// held and replayed across a live-migration pause.
const relayBufferSize = 32 * 1024

// writeAll writes all of p to w, looping until every byte is written or a write
// returns an error. It returns the number of bytes written so a caller
// recovering from a bridge drop can hold and replay the unwritten remainder
// p[n:] on a freshly dialed connection. copyOut uses it to write process output
// to the host conn and copyIn to write host input to the process pipe, so the
// parameter is the wide io.Writer that both a transport.Connection and a pipe
// satisfy.
func writeAll(w io.Writer, p []byte) (int, error) {
	written := 0
	for written < len(p) {
		n, err := w.Write(p[written:])
		// Count n before the error check: per the io.Writer contract these n
		// bytes were written even on a partial-write error, so the caller
		// replays only p[written:] and never re-sends them.
		written += n
		if err != nil {
			return written, err
		}
		if n == 0 {
			// A well-behaved io.Writer never returns (0, nil) with bytes
			// still to write; treat it as a short write (matching io.Copy's
			// contract) so a misbehaving connection cannot spin this loop
			// forever.
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

// errNeedsRedial signals that a copier stopped because the host connection
// dropped while canRedial was set, so the manager should re-dial and resume
// rather than tear the process stdio down. Unlike a real copy/read/write error
// (already logged where it occurred), this is expected control flow during
// every live migration, so it is a plain sentinel rather than a wrapped error
// - matching how io.EOF signals a normal stream end rather than a failure.
var errNeedsRedial = errors.New("stdio: connection dropped, redial needed")

// copyIn copies host input from conn r into the process sink w (a stdin pipe or
// a pty master). It decides by which side failed, mirroring copyOut: a read
// error on r is the host conn, so a bridge drop during a live migration
// returns errNeedsRedial when canRedial is set so the manager re-dials
// so the next call resumes reading from the fresh conn; a write error on w is
// the process closing its stdin (EPIPE), which is a normal end and returns nil
// even while the bridge is down (otherwise a post-resume stdin close would be
// mistaken for a pause and spin the manager re-dialing). A bridge-down read
// yields no bytes, so there is nothing to hold. On host EOF and any other
// unexpected error it returns nil.
//
// sig distinguishes our own wake from a real end of stream: wakeParkedCopiers
// unblocks this copier with shutdown(2), which surfaces as EOF and is otherwise
// indistinguishable from the host closing stdin.
func copyIn(w io.Writer, r io.Reader, name string, canRedial bool, sig *redialSignal) error {
	l := logrus.WithField("file", name)
	buf := make([]byte, relayBufferSize)
	for {
		nr, rerr := r.Read(buf)
		if nr > 0 {
			if _, werr := writeAll(w, buf[:nr]); werr != nil {
				// A pipe write failure is the process closing its stdin: a
				// normal end, never a pause, even while the bridge is down.
				l.WithError(werr).Error("opengcs::stdio::copyIn - error writing to process input")
				return nil
			}
		}
		if rerr != nil {
			// A forced CloseRead also returns EOF, so consult the round signal
			// first: mistaking a wake for a real close would shut the process's
			// stdin pipe permanently.
			if canRedial && sig != nil && sig.raised() {
				return errNeedsRedial
			}
			// io.EOF is the host closing stdin: a normal end.
			if rerr == io.EOF {
				if canRedial {
					// A connection severed by migration reports ECONNRESET and
					// takes the redial branch below, so reaching EOF here should
					// only ever be a genuine host close. Logged because if a dead
					// connection can reach it, the caller closes the process's
					// stdin pipe and no redial brings it back.
					l.Info("opengcs::stdio::copyIn - stdin EOF while a redial was possible")
				}
				return nil
			}
			if canRedial {
				return errNeedsRedial
			}
			l.WithError(rerr).Error("opengcs::stdio::copyIn - error reading input")
			return nil
		}
	}
}

// copyOut copies process output from pipe r into host conn c using a retained
// buffer. pending holds the in-flight remainder from a prior bridge-drop pause
// and is replayed onto c first. On a bridge-down write failure it returns the
// still-unwritten bytes with err=errNeedsRedial so the manager re-dials and the
// next call replays them on the fresh conn (no relay-level drop). On pipe EOF
// (the process closed the stream) it performs the existing clean socket shutdown.
func copyOut(c transport.Connection, r io.Reader, name string, pending []byte, canRedial bool) (held []byte, err error) {
	l := logrus.WithField("file", name)

	// copyDone is set when the copy phase is over for a non-pause reason (pipe
	// EOF or a real copy error); the relay then cleanly shuts the socket down.
	// A pause returns early, before any shutdown, so the manager can re-dial and
	// the next call replays the held remainder.
	copyDone := false

	// Replay any in-flight remainder retained from a prior pause first, so the
	// bytes already read from the pipe before the bridge dropped are not lost.
	if len(pending) > 0 {
		if n, werr := writeAll(c, pending); werr != nil {
			if canRedial {
				return bytes.Clone(pending[n:]), errNeedsRedial
			}
			l.WithError(werr).Error("opengcs::stdio::copyOut - error replaying retained output")
			copyDone = true
		}
	}

	buf := make([]byte, relayBufferSize)
	for !copyDone {
		nr, rerr := r.Read(buf)
		if nr > 0 {
			if nw, werr := writeAll(c, buf[:nr]); werr != nil {
				if canRedial {
					return bytes.Clone(buf[nw:nr]), errNeedsRedial
				}
				l.WithError(werr).Error("opengcs::stdio::copyOut - error copying from pipe")
				break
			}
		}
		if rerr != nil {
			// Deadline expiry is the relay's own wake, not a real error. Return
			// before the clean-close block below, which waits for a peer EOF that
			// a dead conn never sends. Nothing is held: a timed-out read yields
			// no bytes.
			if errors.Is(rerr, os.ErrDeadlineExceeded) {
				return nil, errNeedsRedial
			}
			if rerr != io.EOF {
				l.WithError(rerr).Error("opengcs::stdio::copyOut - error reading from pipe")
			}
			break
		}
	}

	// Shut down the write end of the socket, then read a byte (which should
	// yield EOF) to wait for the other endpoint to finish reading and close
	// the connection.
	if cerr := c.CloseWrite(); cerr == nil {
		var b [1]byte
		_, cerr = c.Read(b[:])
		if cerr == nil {
			cerr = errors.New("unexpected data in socket")
		}
		if cerr != io.EOF { //nolint:errorlint
			l.WithError(cerr).Error("opengcs::stdio::copyOut - error reading for clean close")
		}
	} else {
		l.WithError(cerr).Error("opengcs::stdio::copyOut - error shutting down socket")
	}
	if cerr := c.Close(); cerr != nil {
		l.WithError(cerr).Error("opengcs::stdio::copyOut - error closing socket")
	}
	return nil, nil
}
