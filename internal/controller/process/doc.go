//go:build windows && (lcow || wcow)

// Package process provides a controller for managing individual process
// (init or exec) instances within a container. It handles the full lifecycle
// from creation through exit, including IO plumbing, signal delivery, and exit
// status reporting.
//
// # Lifecycle
//
// A controller created via [New] follows the live-creation path:
//
//	┌───────────────────┐
//	│  StateNotCreated  │
//	└────────┬──────────┘
//	         │ Create
//	         ▼
//	┌───────────────────┐
//	│   StateCreated    │── Start fails / Kill / Delete──┐
//	└────────┬──────────┘                                │
//	         │ Start ok                                  │
//	         ▼                                           │
//	┌───────────────────┐                                │
//	│   StateRunning    │──── process exits / Kill ──────┤
//	└───────────────────┘                                │
//	                                                     ▼
//	                                           ┌───────────────────┐
//	                                           │  StateTerminated  │
//	                                           └───────────────────┘
//
// Live migration adds two branches. The destination rehydrates a process via
// [Import] into StateDestinationMigrating; [Controller.Patch] rebinds its IO
// without leaving that state, and [Controller.Resume] reattaches the live
// process (→ StateRunning) or [Controller.AbortMigrated] tears it down
// (→ StateTerminated). The source freezes a running process via [Controller.Save]
// into StateSourceMigrating; [Controller.Resume] rolls it back (→ StateRunning),
// or its exit on source VM teardown terminates it (→ StateTerminated):
//
//	             destination                          source
//	  Patch (rebinds IO, stays)
//	  ┌──────────────┐
//	  ▼              │
//	┌───────────────────────────┐        ┌──────────────────────┐
//	│ StateDestinationMigrating │        │ StateSourceMigrating │
//	└───┬───────────────────┬───┘        └───┬──────────────┬───┘
//	    │ Resume            │ Abort          │ Resume       │ exit
//	    ▼                   ▼                ▼              ▼
//	StateRunning     StateTerminated   StateRunning   StateTerminated
//
//	  - [Controller.Create] sets up upstream IO connections and stores the
//	    process spec. The controller transitions from StateNotCreated to
//	    StateCreated.
//	  - [Controller.Start] launches the process inside the hosting system
//	    and spawns a background goroutine to monitor exit. The controller
//	    transitions from StateCreated to StateRunning.
//	  - [Controller.Kill] delivers a signal to a running process or
//	    terminates a process that has not yet started running.
//	  - [Controller.Delete] prepares the process for removal from the
//	    container's process table. For a created-but-never-started process,
//	    it transitions to StateTerminated and releases its IO resources.
//	  - [Controller.Wait] blocks until the process exits or the context
//	    is cancelled.
//	  - [Controller.Status] returns the current containerd-compatible state
//	    of the process.
//
// # Exit Handling
//
// When a process is started, a background goroutine waits for the process
// to exit, records the exit code and timestamp, drains all IO copies, and
// publishes a TaskExit event when the caller supplies an events channel (an
// init process passes none, since its exit is reported by its owning
// container). The exitedCh channel is closed once all cleanup is complete,
// unblocking any [Controller.Wait] callers.
package process
