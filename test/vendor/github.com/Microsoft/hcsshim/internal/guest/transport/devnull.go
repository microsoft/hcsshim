//go:build linux
// +build linux

package transport

import (
	"os"

	"github.com/sirupsen/logrus"
)

// DevNullTransport is a transport that will:
//
// For reads: return either closed or EOF for as appropriate.
// For writers: return either closed or throw away the write as appropriate.
//
// The DevNullTransport is used as the container logging transport when stdio
// access is denied. It is also used for non-terminal external processes (aka
// a process run in the UVM) when stdio access is denied.
type DevNullTransport struct{}

func (t *DevNullTransport) Dial(fd uint32) (Connection, error) {
	logrus.WithFields(logrus.Fields{
		"fd": fd,
	}).Info("opengcs::DevNullTransport::Dial")

	return newDevNullConnection(), nil
}

// devNullConnection is the heart of our new transport. A devNullConnection
// contains two file descriptors. One for read, and one for write. We need to
// file descriptors as the code using a transport is written with the
// expectation of a duplex connection where read and write can be closed
// independent of one another. The protocol that uses the connection requires
// a duplex connection in order to work correctly.
//
// The original design of devNullConnection didn't use os.File and instead
// emulated the required behavior and only returned a os.File from the `File`
// function. However, the amount of state required to manage that was somewhat
// high and the logic to follow was somewhat convoluted. Using two file handles
// one read, one write that have /dev/null open results in a much cleaner
// design and easier to understand semantics. We want to mirror the sematics of
// /dev/null as a duplex connection, this is the simplest, most understandable
// way to achieve that.
type devNullConnection struct {
	read  *os.File
	write *os.File
}

func newDevNullConnection() *devNullConnection {
	r, _ := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	w, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)

	return &devNullConnection{read: r, write: w}
}

func (c *devNullConnection) Close() error {
	err1 := c.read.Close()
	err2 := c.write.Close()

	if err1 != nil {
		return err1
	}

	if err2 != nil {
		return err2
	}

	return nil
}

func (c *devNullConnection) CloseRead() error {
	return c.read.Close()
}

func (c *devNullConnection) CloseWrite() error {
	return c.write.Close()
}

func (c *devNullConnection) Read(buf []byte) (int, error) {
	return c.read.Read(buf)
}

func (c *devNullConnection) Write(buf []byte) (int, error) {
	return c.write.Write(buf)
}

// File() is where our lack of a real duplex connection is problematic. Code
// that uses a connection sidesteps the connection abstraction and asks for
// "the file" directly. With vsock, which is an actual duplex connection, this
// isn't actually a problem as a vsock connection is duplex. It dups the
// connection and returns and everything works just fine.
//
// Our emulating a duplex connection could be problematic depending on what
// code using the os.File returned from File() expects it semantics to be.
// In particular, with a dup like vsock does, if you close the os.File returned
// from File() it closes the connection. That isn't the case with our emulated
// duplex connection as closing the os.File that devNullConneciton.File()
// returns has no impact on the `read` and `write` file handles that are used
// during Read/Write calls.
//
// In the current usage in GCS, this isn't problematic but could becomes so.
// If you end up here because of a bug, here is the statement from the author
// about ways to fix this problem.
//
//  1. File() is a leaky abstraction. A transport should be required to be duplex
//     so, having a single `File()` call that allows the caller to reach into the
//     guts of a transport is bad and should be done away with. All usage of a
//     transport/connection should go through their abstraction, not bypass it
//     when bypassing is more convenient.
//  2. If File() didn't return a concrete type like os.File and instead returned
//     an interface, then a more complicated version of devNullConnection that
//     doesn't use a file handle could be written and the object that we return
//     could be able to correctly handle the emulation of a duplex object.
//
// Both of the above are somewhat large changes and were not done at the time
// the initial version of DevNullTransport and devNullConnection were done. It
// was decided to leave the leaky abstraction in place as the code that uses
// transports is fiddly and error-prone based on not very well documentated
// assumptions. If you are considering taking on either 1 or 2 from above, you
// should have plenty of time on your hands as extensive testing will be
// required and the introduction of bugs both subtle and obvious is a likely
// possibility. Aka, reworking this code to remove the leaky abstraction is more
// than a refactoring and should be done with extreme care.
func (c *devNullConnection) File() (*os.File, error) {
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	return f, nil
}
