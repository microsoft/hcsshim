// Notifications that `cow.Containers` can raise and Notify clients about
//
// generalized from `internal/gcs/protocol.go` and `internal/guest/prot/protocol.go`
package notifications

import (
	"errors"
	"fmt"

	"github.com/Microsoft/hcsshim/internal/queue"
)

type MessageType string

type Message interface {
	fmt.Stringer
	Type() MessageType
}

// trivial implementation, with no extra info
type message string

// todo (helsaawy): evalute string overhead is more than `iota` with "const" LUT from into to string
const (
	// None indicates a no-op notification
	None = message("None")
	// Unknown indicates an unknown notification
	Unknown = message("Unknown")
	// GracefulExit indicates a graceful container exit notification
	GracefulExit = message("GracefulExit")
	// ForcedExit indicates a forced container exit notification
	ForcedExit = message("ForcedExit")
	// UnexpectedExit indicates an unexpected container exit notification
	UnexpectedExit = message("UnexpectedExit")
	// Reboot indicates a container reboot notification
	Reboot = message("Reboot")
	// OomEvent indicates an Out of Memory (OOM) notification
	Oom = message("OOM")
	// Constructed indicates a constructed notification
	Constructed = message("Constructed") // todo (helsaawy): what is this used for? inherited from HCS?
	// Creates indicates a container creation notification
	Created = message("Created")
	//ExecCreated indicated an exec creation notification
	ExecCreated = message("ExecCreated")
	// Started indicates a container started notification
	Started = message("Started")
	// ExceStarted indicates an exec started within a container
	ExecStarted = message("ExecStarted")
	// Deleted indicated a container deleted notification
	Deleted = message("Deleted")
	// Paused indicates a container paused notification
	Paused = message("Paused")
	// Checkpoint indicates a notification of a container checkpoint
	Checkpoint = message("Checkpoint")
)

func (n message) String() string {
	return string(n)
}

func (n message) Type() MessageType {
	return MessageType(n)
}

func FromString(n string) Message {
	if len(n) == 0 {
		return None
	}
	return message(n)
}

var ErrNotificationsNotSupported = errors.New("notifications not supported")

// No-Op implementation of Notifications for the cow.Containers interface
type NullNotifications struct{}

func (*NullNotifications) Notifications() (*queue.MessageQueue, error) {
	// pointer receiver to match most cow.Container implementations
	return nil, ErrNotificationsNotSupported
}
