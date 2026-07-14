//go:build windows && wcowprocess

package processisolated

// State represents the lifecycle state of a host WCOW container.
type State int32

const (
	StateNotCreated State = iota
	StateCreated
	StateRunning
	StateStopped
	StateInvalid
)

// String returns a human-readable representation of the State.
func (s State) String() string {
	switch s {
	case StateNotCreated:
		return "NotCreated"
	case StateCreated:
		return "Created"
	case StateRunning:
		return "Running"
	case StateStopped:
		return "Stopped"
	case StateInvalid:
		return "Invalid"
	default:
		return "Unknown"
	}
}
