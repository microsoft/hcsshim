package transport

import "io"

// MockTransport is a mock implementation of Transport.
type MockTransport struct {
	// Channel sends connections to the "server" once the "client" has
	// connected.
	Channel chan *MockConnection
}

// Dial ignores the port, and returns a MockTransport struct.
func (t *MockTransport) Dial(_ uint32) (Connection, error) {
	csr, csw := io.Pipe()
	scr, scw := io.Pipe()
	t.Channel <- &MockConnection{
		PipeReader: csr,
		PipeWriter: scw,
	}
	return &MockConnection{
		PipeReader: scr,
		PipeWriter: csw,
	}, nil
}

// MockConnection is a mock implementation of Connection.
type MockConnection struct {
	*io.PipeReader
	*io.PipeWriter
}

// Close marks the connection as closed, and closes the read and write
// channels.
func (c *MockConnection) Close() error {
	c.PipeReader.Close()
	c.PipeWriter.Close()
	return nil
}

// CloseRead closes the read channel.
func (c *MockConnection) CloseRead() error {
	return c.PipeReader.Close()
}

// CloseWrite closes the write channel.
func (c *MockConnection) CloseWrite() error {
	return c.PipeWriter.Close()
}
