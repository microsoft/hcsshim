package bridge

import (
	"encoding/binary"
	"io"

	"github.com/sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
)

const (
	commandPort uint32 = 0x40000000
)

func (b *bridge) createAndConnectCommandConn() (transport.Connection, error) {
	conn, err := b.tport.Dial(commandPort)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating the command Connection")
	}
	logrus.Info("successfully connected to the HCS via HyperV_Socket\n")
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
	logrus.Infof("READ: %s\n", b)
	return b, header, nil
}

// sendMessageBytes sends a header with the given messageType and messageID, and a
// message body with the given str as content, to conn.
func sendMessageBytes(conn transport.Connection, messageType prot.MessageIdentifier, messageID prot.SequenceID, b []byte) error {
	header := prot.MessageHeader{
		Type: messageType,
		ID:   messageID,
		Size: uint32(len(b) + prot.MessageHeaderSize),
	}
	if err := binary.Write(conn, binary.LittleEndian, &header); err != nil {
		return errors.Wrap(err, "failed writing message header")
	}

	if _, err := conn.Write(b); err != nil {
		return errors.Wrap(err, "failed writing message payload")
	}
	logrus.Infof("SENT: %s\n", b)
	return nil
}

// sendResponseBytes behaves the same as sendMessageBytes, except it converts
// messageType to its response version before sending.
func sendResponseBytes(conn transport.Connection, messageType prot.MessageIdentifier, messageID prot.SequenceID, b []byte) error {
	return sendMessageBytes(conn, prot.GetResponseIdentifier(messageType), messageID, b)
}

// connectStdio returns new transport.Connection instances, one for each
// stdio pipe to be used. If CreateStd*Pipe for a given pipe is false, the
// given Connection is set to nil.
func connectStdio(tport transport.Transport, params prot.ProcessParameters, settings prot.ExecuteProcessVsockStdioRelaySettings) (s *stdio.ConnectionSet, err error) {
	s = &stdio.ConnectionSet{}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	if params.CreateStdInPipe {
		s.In, err = tport.Dial(settings.StdIn)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdin Connection")
		}
	}
	if params.CreateStdOutPipe {
		s.Out, err = tport.Dial(settings.StdOut)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
	}
	if params.CreateStdErrPipe {
		s.Err, err = tport.Dial(settings.StdErr)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
	}
	return s, nil
}
