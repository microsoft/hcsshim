//go:build windows

package vm

import (
	"fmt"
	"sync/atomic"
)

// State represents the current state of the VM lifecycle.
// The VM progresses through states in the following order:
// StateNotCreated -> StateCreated -> StateRunning -> StateStopped
type State int32

const (
	// StateNotCreated indicates the VM has not been created yet.
	// This is the initial state when a Controller is first instantiated.
	// Valid transitions: StateNotCreated -> StateCreated (via CreateVM)
	StateNotCreated State = iota

	// StateCreated indicates the VM has been created but not started.
	// Valid transitions: StateCreated -> StateRunning (via StartVM)
	StateCreated

	// StateRunning indicates the VM has been started and is running.
	// The guest OS is running and the Guest Compute Service (GCS) connection
	// is established.
	// Valid transitions: StateRunning -> StateStopped (when VM exits or is terminated)
	StateRunning

	// StateStopped indicates the VM has exited or been terminated.
	// This is a terminal state - once stopped, the VM cannot be restarted.
	// No further state transitions are possible.
	StateStopped
)

// String returns a human-readable string representation of the VM State.
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
	default:
		return "Unknown"
	}
}

// atomicState is a concurrency-safe VM state holder backed by an atomic int32.
// All reads and writes go through atomic operations, so no mutex is required
// for state itself.
type atomicState struct {
	v atomic.Int32
}

// load returns the current State with an atomic read.
func (a *atomicState) load() State {
	return State(a.v.Load())
}

// store unconditionally sets the state with an atomic write.
func (a *atomicState) store(s State) {
	a.v.Store(int32(s))
}

// transition atomically moves from `from` to `to` using a compare-and-swap.
// It returns an error if the current state is not `from`, leaving the state
// unchanged. This prevents two concurrent callers from both believing they
// performed the same transition.
func (a *atomicState) transition(from, to State) error {
	if !a.v.CompareAndSwap(int32(from), int32(to)) {
		return fmt.Errorf("unexpected VM state: want %s, got %s", from, a.load())
	}
	return nil
}
