package bridge

import (
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/Microsoft/opengcs/service/libs/commonutils"
	"io"
)

const (
	commandPort uint32 = 0x40000000
)

type StdioConnSet struct {
	In  transport.Connection
	Out transport.Connection
	Err transport.Connection
}

func (s *StdioConnSet) Close() error {
	var err error
	if s.In != nil {
		if cerr := s.In.CloseRead(); err == nil {
			err = errors.Wrap(cerr, "failed CloseRead on stdin")
		}
		if cerr := s.In.Close(); err == nil {
			err = errors.Wrap(cerr, "failed Close on stdin")
		}
	}
	if s.Out != nil {
		if cerr := s.Out.Close(); err == nil {
			err = errors.Wrap(cerr, "failed Close on stdout")
		}
	}
	if s.Err != nil {
		if cerr := s.Err.Close(); err == nil {
			err = errors.Wrap(cerr, "failed Close on stderr")
		}
	}
	return err
}

func (b *bridge) createAndConnectCommandConn() (transport.Connection, error) {
	conn, err := b.tport.Dial(commandPort)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating the command Connection")
	}
	utils.LogMsg("successfully connected to the HCS via HyperV_Socket\n")
	return conn, nil
}

// readString reads a message string from the given Connection, assuming the
// next byte to be read is the beginning of a MessageHeader.
func readMessage(conn transport.Connection) ([]byte, *prot.MessageHeader, error) {
	header := &prot.MessageHeader{}
	if err := binary.Read(conn, binary.LittleEndian, header); err != nil {
		return nil, nil, errors.Wrap(err, "failed reading message header")
	}
	b := make([]byte, header.Size-prot.MessageHeaderSize)
	if _, err := io.ReadFull(conn, b); err != nil {
		return nil, nil, errors.Wrap(err, "failed reading message payload")
	}
	utils.LogMsgf("READ: %s\n", b)
	return b, header, nil
}

// sendMessageBytes sends a header with the given messageType and messageId, and a
// message body with the given str as content, to conn.
func sendMessageBytes(conn transport.Connection, messageType prot.MessageIdentifier, messageId prot.SequenceId, b []byte) error {
	header := prot.MessageHeader{
		Type: messageType,
		Id:   messageId,
		Size: uint32(len(b) + prot.MessageHeaderSize),
	}
	if err := binary.Write(conn, binary.LittleEndian, &header); err != nil {
		return errors.Wrap(err, "failed writing message header")
	}

	if _, err := conn.Write(b); err != nil {
		return errors.Wrap(err, "failed writing message payload")
	}
	utils.LogMsgf("SENT: %s\n", b)
	return nil
}

// sendResponseBytes behaves the same as sendMessageBytes, except it converts
// messageType to its response version before sending.
func sendResponseBytes(conn transport.Connection, messageType prot.MessageIdentifier, messageId prot.SequenceId, b []byte) error {
	return sendMessageBytes(conn, prot.GetResponseIdentifier(messageType), messageId, b)
}

// createAndConnectStdio returns new transport.Connection structs, one for each
// stdio pipe to be used. If CreateStd*Pipe for a given pipe is false, the
// given Connection is set to nil. After creating each transport.Connection, it
// calls Connect on it.
func createAndConnectStdio(tport transport.Transport, params prot.ProcessParameters, settings prot.ExecuteProcessVsockStdioRelaySettings) (*StdioConnSet, error) {
	var err error
	var stdin transport.Connection
	if params.CreateStdInPipe {
		stdin, err = tport.Dial(settings.StdIn)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdin Connection")
		}
	}
	var stdout transport.Connection
	if params.CreateStdOutPipe {
		stdout, err = tport.Dial(settings.StdOut)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
	}
	var stderr transport.Connection
	if params.CreateStdErrPipe {
		stderr, err = tport.Dial(settings.StdErr)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
	}
	return &StdioConnSet{In: stdin, Out: stdout, Err: stderr}, nil
}
