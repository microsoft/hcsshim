package bridge

import "strconv"

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

type MessageIdentifier uint32

func NewIdentifier(t Type, id ID) MessageIdentifier {
	return MessageIdentifier(t) | MessageIdentifier(CategoryContainer) | MessageIdentifier(id) | MessageIdentifier(VersionV1)
}

func (mi MessageIdentifier) AsType(t Type) MessageIdentifier {
	return MessageIdentifier(t) | (mi &^ typeMask)
}

func (mi MessageIdentifier) AsResponse() MessageIdentifier {
	return mi.AsType(TypeResponse)
}

// cannot call this `type`, since its a keyword
// so name all getters as `msg*`
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

type Category uint32

const CategoryContainer Category = 0x00100000

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

func (id ID) NotifyString() string {
	switch id {
	case NotifyContainer:
		return "Container"
	default:
		return "<unknown notification>"
	}
}

type Version uint32

const VersionV1 Version = 0x1
