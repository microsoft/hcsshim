package transport

import (
	"io"
	"os"
)

// TODO: specialized [Transport] and [Connection] for [io.Reader]/[io.Writer] instead of both,
// so either stdin or stdout/stderr can be specialized without affecting the other.
// i.e., don't use [multiWriter] for stdin.

// Transport is the interface defining a method of transporting data in a
// connection-like way.
// Examples of a Transport implementation could be:
//
//	-Hyper-V socket transport
//	-TCP/IP socket transport
//	-Mocked-out local transport
type Transport interface {
	// Dial takes a port number and returns a connected connection.
	Dial(port uint32) (Connection, error)
}

// Connection is the interface defining a data connection, such as a socket or
// a mocked implementation.
type Connection interface {
	io.ReadWriteCloser
	CloseRead() error
	CloseWrite() error
	File() (*os.File, error)
}
