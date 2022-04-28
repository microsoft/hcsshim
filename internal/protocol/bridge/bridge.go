package bridge

import (
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"unsafe"
)

// MessageHeader is the common header present in all communications messages.
type MessageHeader struct {
	Type MessageIdentifier
	Size uint32
	ID   SequenceID
}

const (
	// MessageHeaderSize is the size in bytes of the MessageHeader struct.
	MessageHeaderSize = int(unsafe.Sizeof(MessageHeader{}))

	_mhTypeOff = unsafe.Offsetof(MessageHeader{}.Type)
	_mhSizeOff = unsafe.Offsetof(MessageHeader{}.Size)
	_mhIDOff   = unsafe.Offsetof(MessageHeader{}.ID)
)

func NewHeader(t MessageIdentifier, s int, id int64) *MessageHeader {
	return &MessageHeader{
		Type: t,
		Size: uint32(s),
		ID:   SequenceID(id),
	}
}

func (h *MessageHeader) FromBytes(b []byte) (err error) {
	if len(b) < MessageHeaderSize {
		return fmt.Errorf("cannot read message header from buffer: %w", io.ErrShortBuffer)
	}
	b = b[:MessageHeaderSize]

	h.Type = MessageIdentifier(binary.LittleEndian.Uint32(b[_mhTypeOff:]))
	h.Size = binary.LittleEndian.Uint32(b[_mhSizeOff:])
	h.ID = SequenceID(binary.LittleEndian.Uint64(b[_mhIDOff:]))

	return nil
}

func (h *MessageHeader) ToBytes(b []byte) error {
	if len(b) < MessageHeaderSize {
		return fmt.Errorf("cannot write message header from buffer: %w", io.ErrShortBuffer)
	}
	b = b[:MessageHeaderSize]

	binary.LittleEndian.PutUint32(b[_mhTypeOff:], uint32(h.Type))
	binary.LittleEndian.PutUint64(b[_mhSizeOff:], uint64(h.Size))
	binary.LittleEndian.PutUint32(b[_mhIDOff:], uint32(h.ID))

	return nil
}

/*
bridge message identifiers in message header:

+---+----+-----+----+
| T | CC | III | VV |
+---+----+-----+----+

T	4 Bits		Type
CC	8 Bits		Category
III	12 Bits		Message Id
VV	8 Bits		Version

Type:
	None			0x0
	Request			0x1
	Response		0x2
	Notify 			0x3

Category:
	None			0x00
	ComputeSystem 	0x01

Message ID:
	request, response, or notification type

Version:
	v1				0x01
*/

const (
	idShift   = 8
	typeShift = 28

	typeMask     = 0xf0000000
	categoryMask = 0x0ff00000
	idMask       = 0x000fff00
	versionMask  = 0x000000ff
)

// MessageIdentifier is part of the message header that provides information about the
// request.
type MessageIdentifier uint32

func NewIdentifier(t Type, id ID) MessageIdentifier {
	return MessageIdentifier(t) | MessageIdentifier(CategoryContainer) | MessageIdentifier(id) | MessageIdentifier(VersionV1)
}

// ChangeType changes the message type of the MessageIdentifier
func (mi MessageIdentifier) ChangeType(t Type) MessageIdentifier {
	return MessageIdentifier(t) | (mi &^ typeMask)
}

// ToResponse changes the message type to TypeResponse
func (mi MessageIdentifier) ToResponse() MessageIdentifier {
	return mi.ChangeType(TypeResponse)
}

func (mi MessageIdentifier) Type() Type {
	return Type(mi & typeMask)
}

func (mi MessageIdentifier) Category() Category {
	return Category(mi & categoryMask)
}

func (mi MessageIdentifier) ID() ID {
	return ID(mi & idMask)
}

func (mi MessageIdentifier) Version() Version {
	return Version(mi & versionMask)
}

func (mi MessageIdentifier) String() string {
	t := mi.Type()
	id := mi.ID()
	s := ""
	switch t {
	case TypeRequest, TypeResponse:
		s = id.RPCString()
	case TypeNotify:
		s = id.NotifyString()
	default:
		s = id.String()
	}

	return t.String() + "(" + s + ")"
}

// Type of the message.
type Type uint32

const (
	TypeNone Type = iota << typeShift
	TypeRequest
	TypeResponse
	TypeNotify
)

func (t Type) String() string {
	switch t {
	case TypeRequest:
		return "Request"
	case TypeResponse:
		return "Response"
	case TypeNotify:
		return "Notify"
	default:
		return "0x" + strconv.FormatUint(uint64(t), 16)
	}
}

// Category namespaces the identifier for different groups of messages.
type Category uint32

const CategoryContainer Category = 0x00100000

// ID is identifies which request, response, or notification the message is for.
type ID uint32

const (
	IDNone ID = iota << idShift

	// for request and response message types

	RPCCreate
	RPCStart
	RPCShutdownGraceful
	RPCShutdownForced
	RPCExecuteProcess
	RPCWaitForProcess
	RPCSignalProcess
	RPCResizeConsole
	RPCGetProperties
	RPCModifySettings
	RPCNegotiateProtocol
	RPCDumpStacks
	RPCDeleteContainerState
	RPCUpdateContainer
	RPCLifecycleNotification

	// for notify message types

	NotifyContainer = 1 << idShift
)

func (id ID) String() string {
	return "0x" + strconv.FormatUint(uint64(id), 16)
}

// RPCString is the message ID as a string for request- and response-type messages.
func (id ID) RPCString() string {
	switch id {
	case RPCCreate:
		return "Create"
	case RPCStart:
		return "Start"
	case RPCShutdownGraceful:
		return "ShutdownGraceful"
	case RPCShutdownForced:
		return "ShutdownForced"
	case RPCExecuteProcess:
		return "ExecuteProcess"
	case RPCWaitForProcess:
		return "WaitForProcess"
	case RPCSignalProcess:
		return "SignalProcess"
	case RPCResizeConsole:
		return "ResizeConsole"
	case RPCGetProperties:
		return "GetProperties"
	case RPCModifySettings:
		return "ModifySettings"
	case RPCNegotiateProtocol:
		return "NegotiateProtocol"
	case RPCDumpStacks:
		return "DumpStacks"
	case RPCDeleteContainerState:
		return "DeleteContainerState"
	case RPCUpdateContainer:
		return "UpdateContainer"
	case RPCLifecycleNotification:
		return "LifecycleNotification"
	default:
		return "<unknown RPC>"
	}
}

// NotifyString is the message ID as a string for notification-type messages.
func (id ID) NotifyString() string {
	switch id {
	case NotifyContainer:
		return "Container"
	default:
		return "<unknown notification>"
	}
}

// Version is the bridge protocol version.
type Version uint32

const VersionV1 Version = 0x1

// SequenceID is used to correlate requests and responses.
type SequenceID uint64
