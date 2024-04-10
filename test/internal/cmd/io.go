//go:build windows

package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Microsoft/hcsshim/internal/cmd"
)

type BufferedIO struct {
	in       *bytes.Buffer
	out, err bytes.Buffer
}

func NewBufferedIOFromString(in string) *BufferedIO {
	b := NewBufferedIO()
	b.in = bytes.NewBufferString(in)

	return b
}

func NewBufferedIO() *BufferedIO {
	return &BufferedIO{}
}

func (b *BufferedIO) Output() (_ string, err error) {
	o := b.out.String()
	if e := b.err.String(); len(e) != 0 {
		err = errors.New(e)
	}

	return o, err
}

func (b *BufferedIO) TestOutput(tb testing.TB, out string, err error) {
	tb.Helper()

	outGot, errGot := b.Output()
	if !errors.Is(errGot, err) {
		tb.Fatalf("got stderr: %v; wanted: %v", errGot, err)
	}

	out = strings.ToLower(strings.TrimSpace(out))
	outGot = strings.ToLower(strings.TrimSpace(outGot))
	if diff := cmp.Diff(out, outGot); diff != "" {
		tb.Fatalf("stdout mismatch (-want +got):\n%s", diff)
	}
}

func (b *BufferedIO) TestStdOutContains(tb testing.TB, want, notWant []string) {
	tb.Helper()

	outGot, err := b.Output()
	if err != nil {
		tb.Fatalf("got stderr: %v", err)
	}

	tb.Logf("searching stdout for substrings\nstdout:\n%s\nwanted substrings:\n%q\nunwanted substrings:\n%q", outGot, want, notWant)

	outGot = strings.ToLower(outGot)

	for _, s := range want {
		if !strings.Contains(outGot, strings.ToLower(s)) {
			tb.Errorf("stdout does not contain substring:\n%s", s)
		}
	}

	for _, s := range notWant {
		if strings.Contains(outGot, strings.ToLower(s)) {
			tb.Errorf("stdout contains substring:\n%s", s)
		}
	}

	// FailNow() to match behavior of [TestOutput]
	if tb.Failed() {
		tb.FailNow()
	}
}

func (b *BufferedIO) AddToCmd(c *cmd.Cmd) {
	if b == nil {
		return
	}

	if c.Stdin == nil && b.in != nil {
		c.Stdin = b.in
	}
	if c.Stdout == nil {
		c.Stdout = &b.out
	}
	if c.Stderr == nil {
		c.Stderr = &b.err
	}
}
