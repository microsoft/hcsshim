//go:build linux
// +build linux

package stdio

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/transport"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// bridgeDown reports whether the GCS bridge to the host is currently down (for
// example because a live migration is tearing down and re-establishing the
// vsock connections). The stdio relays consult it to tell a live-migration
// disconnect (pause and resume) apart from a normal process-driven copy error
// (tear down). It is set by cmd/gcs around the bridge reconnect loop.
var bridgeDown atomic.Bool

// SetBridgeDown records whether the host bridge is currently down. cmd/gcs sets
// it true when ListenAndServe returns (and it is not a shutdown) and false once
// the bridge has reconnected.
func SetBridgeDown(v bool) {
	bridgeDown.Store(v)
}

const (
	// redialInterval is the delay between attempts to re-establish the stdio
	// connections after a bridge drop.
	redialInterval = 100 * time.Millisecond
	// maxRedialAttempts bounds how long a relay waits for the bridge to come
	// back before giving up and tearing the process stdio down.
	maxRedialAttempts = 60
)

// redialWithRetry re-establishes a ConnectionSet using the provided redial
// closure, retrying up to maxRedialAttempts with redialInterval between
// attempts. It returns the new set on success or the last error after the
// attempts are exhausted.
func redialWithRetry(redial func() (*ConnectionSet, error)) (*ConnectionSet, error) {
	var err error
	for i := 0; i < maxRedialAttempts; i++ {
		var ns *ConnectionSet
		ns, err = redial()
		if err == nil {
			return ns, nil
		}
		time.Sleep(redialInterval)
	}
	return nil, err
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

// copyIn copies host input from conn r into the process sink w (a stdin pipe or
// a pty master). It decides by which side failed, mirroring copyOut: a read
// error on r is the host conn, so a bridge drop during a live migration
// (pauseOnError and bridgeDown) returns true (a pause) and the manager re-dials
// so the next call resumes reading from the fresh conn; a write error on w is
// the process closing its stdin (EPIPE), which is a normal end and returns false
// even while the bridge is down (otherwise a post-resume stdin close would be
// mistaken for a pause and spin the manager re-dialing). A bridge-down read
// yields no bytes, so there is nothing to hold. On host EOF and any other
// unexpected error it returns false.
func copyIn(w io.Writer, r io.Reader, name string, pauseOnError bool) (paused bool) {
	l := logrus.WithField("file", name)
	buf := make([]byte, relayBufferSize)
	for {
		nr, rerr := r.Read(buf)
		if nr > 0 {
			if _, werr := writeAll(w, buf[:nr]); werr != nil {
				// A pipe write failure is the process closing its stdin: a
				// normal end, never a pause, even while the bridge is down.
				l.WithError(werr).Error("opengcs::stdio::copyIn - error writing to process input")
				return false
			}
		}
		if rerr != nil {
			// io.EOF is the host closing stdin: a normal end.
			if rerr == io.EOF {
				return false
			}
			if pauseOnError && bridgeDown.Load() {
				// A conn read error while the bridge is down is the
				// live-migration drop: pause so the manager re-dials.
				return true
			}
			l.WithError(rerr).Error("opengcs::stdio::copyIn - error reading input")
			return false
		}
	}
}

// copyOut copies process output from pipe r into host conn c using a retained
// buffer. pending holds the in-flight remainder from a prior bridge-drop pause
// and is replayed onto c first. On a bridge-down write failure it returns the
// still-unwritten bytes as held and paused=true so the manager re-dials and the
// next call replays them on the fresh conn (no relay-level drop). On pipe EOF
// (the process closed the stream) it performs the existing clean socket shutdown
// and returns paused=false.
func copyOut(c transport.Connection, r io.Reader, name string, pending []byte, pauseOnError bool) (held []byte, paused bool) {
	l := logrus.WithField("file", name)

	// copyDone is set when the copy phase is over for a non-pause reason (pipe
	// EOF or a real copy error); the relay then cleanly shuts the socket down.
	// A pause returns early, before any shutdown, so the manager can re-dial and
	// the next call replays the held remainder.
	copyDone := false

	// Replay any in-flight remainder retained from a prior pause first, so the
	// bytes already read from the pipe before the bridge dropped are not lost.
	if len(pending) > 0 {
		if n, err := writeAll(c, pending); err != nil {
			if pauseOnError && bridgeDown.Load() {
				l.WithError(err).Info("opengcs::stdio::copyOut - pausing on bridge down")
				h := make([]byte, len(pending)-n)
				copy(h, pending[n:])
				return h, true
			}
			l.WithError(err).Error("opengcs::stdio::copyOut - error replaying retained output")
			copyDone = true
		}
	}

	buf := make([]byte, relayBufferSize)
	for !copyDone {
		nr, rerr := r.Read(buf)
		if nr > 0 {
			if nw, werr := writeAll(c, buf[:nr]); werr != nil {
				if pauseOnError && bridgeDown.Load() {
					l.WithError(werr).Info("opengcs::stdio::copyOut - pausing on bridge down")
					h := make([]byte, nr-nw)
					copy(h, buf[nw:nr])
					return h, true
				}
				l.WithError(werr).Error("opengcs::stdio::copyOut - error copying from pipe")
				break
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				l.WithError(rerr).Error("opengcs::stdio::copyOut - error reading from pipe")
			}
			break
		}
	}

	// Shut down the write end of the socket, then read a byte (which should
	// yield EOF) to wait for the other endpoint to finish reading and close
	// the connection.
	if err := c.CloseWrite(); err == nil {
		var b [1]byte
		_, err = c.Read(b[:])
		if err == nil {
			err = errors.New("unexpected data in socket")
		}
		if err != io.EOF { //nolint:errorlint
			l.WithError(err).Error("opengcs::stdio::copyOut - error reading for clean close")
		}
	} else {
		l.WithError(err).Error("opengcs::stdio::copyOut - error shutting down socket")
	}
	if err := c.Close(); err != nil {
		l.WithError(err).Error("opengcs::stdio::copyOut - error closing socket")
	}
	return nil, false
}
