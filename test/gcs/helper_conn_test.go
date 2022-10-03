//go:build linux

package gcs

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/Microsoft/hcsshim/internal/guest/stdio"
	"github.com/Microsoft/hcsshim/internal/guest/transport"
)

const (
	dialRetries = 4
	dialWait    = 50 * time.Millisecond
)

// port numbers to assign to connections.
var (
	_pipes      sync.Map
	_portNumber uint32 = 1
)

type PipeTransport struct{}

var _ transport.Transport = &PipeTransport{}

func (*PipeTransport) Dial(port uint32) (c transport.Connection, err error) {
	for i := 0; i < dialRetries; i++ {
		c, err = getFakeSocket(port)

		if errors.Is(err, unix.ENOENT) {
			// socket hasn't been created
			time.Sleep(dialWait)
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}

	logrus.Debugf("dialed port %d", port)
	return c, nil
}

type fakeIO struct {
	stdin, stdout, stderr *fakeSocket
}

func createStdIO(ctx context.Context, tb testing.TB, con stdio.ConnectionSettings) *fakeIO {
	tb.Helper()
	f := &fakeIO{}
	if con.StdIn != nil {
		f.stdin = newFakeSocket(ctx, tb, *con.StdIn, "stdin")
	}
	if con.StdOut != nil {
		f.stdout = newFakeSocket(ctx, tb, *con.StdOut, "stdout")
	}
	if con.StdErr != nil {
		f.stderr = newFakeSocket(ctx, tb, *con.StdErr, "stderr")
	}

	return f
}

func (f *fakeIO) WriteIn(_ context.Context, tb testing.TB, s string) {
	tb.Helper()
	if f.stdin == nil {
		return
	}

	b := []byte(s)
	n := len(b)

	nn, err := f.stdin.Write(b)
	if err != nil {
		tb.Errorf("write to std in: %v", err)
	}
	if n != nn {
		tb.Errorf("only wrote %d bytes, expected %d", nn, n)
	}
}

func (f *fakeIO) CloseIn(_ context.Context, tb testing.TB) {
	tb.Helper()
	if f.stdin == nil {
		return
	}

	if err := f.stdin.CloseWrite(); err != nil {
		tb.Errorf("close write std in: %v", err)
	}

	if err := f.stdin.Close(); err != nil {
		tb.Errorf("close std in: %v", err)
	}
}

func (f *fakeIO) ReadAllOut(ctx context.Context, tb testing.TB) string {
	tb.Helper()
	return f.stdout.readAll(ctx, tb)
}

func (f *fakeIO) ReadAllErr(ctx context.Context, tb testing.TB) string {
	tb.Helper()
	return f.stderr.readAll(ctx, tb)
}

type fakeSocket struct {
	id   uint32
	n    string
	ch   chan struct{} // closed when dialed (via getFakeSocket)
	r, w *os.File
}

var _ transport.Connection = &fakeSocket{}

func newFakeSocket(_ context.Context, tb testing.TB, id uint32, n string) *fakeSocket {
	tb.Helper()

	_, ok := _pipes.Load(id)
	if ok {
		tb.Fatalf("socket %d already exits", id)
	}

	r, w, err := os.Pipe()
	if err != nil {
		tb.Fatalf("could not create socket: %v", err)
	}

	s := &fakeSocket{
		id: id,
		n:  n,
		r:  r,
		w:  w,
		ch: make(chan struct{}),
	}
	_pipes.Store(id, s)

	return s
}

func getFakeSocket(id uint32) (*fakeSocket, error) {
	f, ok := _pipes.Load(id)
	if !ok {
		return nil, unix.ENOENT
	}

	s := f.(*fakeSocket)
	select {
	case <-s.ch:
	default:
		close(s.ch)
	}

	return s, nil
}

func (s *fakeSocket) Read(b []byte) (int, error) {
	<-s.ch
	return s.r.Read(b)
}

func (s *fakeSocket) Write(b []byte) (int, error) {
	<-s.ch
	return s.w.Write(b)
}

func (s *fakeSocket) Close() (err error) {
	if _, ok := _pipes.LoadAndDelete(s.id); ok {
		return nil
	}

	err = s.r.Close()
	if err := s.w.Close(); err != nil {
		return err
	}

	return err
}

func (s *fakeSocket) CloseRead() error {
	return s.r.Close()
}

func (s *fakeSocket) CloseWrite() error {
	return s.w.Close()
}

func (*fakeSocket) File() (*os.File, error) {
	return nil, errors.New("fakeSocket does not support File()")
}

func (s *fakeSocket) readAll(ctx context.Context, tb testing.TB) string {
	tb.Helper()
	return string(s.readAllByte(ctx, tb))
}

func (s *fakeSocket) readAllByte(ctx context.Context, tb testing.TB) (b []byte) {
	tb.Helper()
	if s == nil {
		return nil
	}

	var err error
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		b, err = io.ReadAll(s)
	}()

	select {
	case <-ch:
		if err != nil {
			tb.Errorf("read all %s: %v", s.n, err)
		}
	case <-ctx.Done():
		tb.Errorf("read all %s context cancelled: %v", s.n, ctx.Err())
	}

	return b
}

func newConnectionSettings(in, out, err bool) stdio.ConnectionSettings {
	c := stdio.ConnectionSettings{}

	if in {
		p := nextPortNumber()
		c.StdIn = &p
	}
	if out {
		p := nextPortNumber()
		c.StdOut = &p
	}
	if err {
		p := nextPortNumber()
		c.StdErr = &p
	}

	return c
}

func nextPortNumber() uint32 {
	return atomic.AddUint32(&_portNumber, 2)
}

func TestFakeSocket(t *testing.T) {
	ctx := context.Background()
	tpt := getTransport()

	ch := make(chan struct{})
	chs := make(chan struct{})
	con := newConnectionSettings(true, true, false)

	// host
	f := createStdIO(ctx, t, con)

	var err error
	go func() { // guest
		defer close(ch)

		var cin, cout transport.Connection
		cin, err = tpt.Dial(*con.StdIn)
		if err != nil {
			t.Logf("dial error %v", err)

			return
		}
		defer cin.Close()

		cout, err = tpt.Dial(*con.StdOut)
		if err != nil {
			t.Logf("dial error %v", err)

			return
		}
		defer cout.Close()

		close(chs)
		var b []byte
		b, err = io.ReadAll(cin)
		if err != nil {
			t.Logf("read all error: %v", err)

			return
		}
		t.Logf("guest read %s", b)

		_, err = cout.Write(b)
		_ = cout.CloseWrite()
	}()

	<-chs // wait for guest to dial
	f.WriteIn(ctx, t, "hello")
	f.WriteIn(ctx, t, " world")
	f.CloseIn(ctx, t)
	t.Logf("host wrote")

	<-ch
	t.Logf("go routine closed")
	if err != nil {
		t.Fatalf("go routine error: %v", err)
	}

	s := f.ReadAllOut(ctx, t)
	t.Logf("host read %q", s)
	if s != "hello world" {
		t.Fatalf("got %q, wanted %q", s, "hello world")
	}
}
