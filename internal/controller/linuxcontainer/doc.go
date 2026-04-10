//go:build windows && lcow

// Package linuxcontainer provides a controller for managing the full lifecycle of
// a single LCOW (Linux Containers on Windows) container running inside a Utility VM (UVM).
//
// It coordinates host-side resource allocation (SCSI layers, Plan9 shares, vPCI devices),
// guest-side container creation via the GCS (Guest Compute Service), and process management.
//
// # Lifecycle
//
// A container follows the state machine below.
//
//	             ┌──────────────────┐
//	             │  StateNotCreated │
//	             └───┬──────────┬───┘
//	   Create ok     │          │ Create fails
//	                 ▼          ▼
//	        ┌──────────────┐  ┌──────────────┐
//	        │ StateCreated │  │ StateInvalid │
//	        └───┬──────┬───┘  └──────────────┘
//	Start ok    │      │ Start fails
//	            ▼      ▼
//	 ┌──────────────┐ ┌──────────────┐
//	 │ StateRunning │ │ StateInvalid │
//	 └──────┬───────┘ └──────────────┘
//	        │ init process exits
//	        ▼
//	 ┌──────────────┐
//	 │ StateStopped │
//	 └──────────────┘
//
// State descriptions:
//
//   - [StateNotCreated]: initial state; no resources have been allocated.
//   - [StateCreated]: after [Controller.Create] succeeds; host-side resources are
//     allocated, the GCS container exists, and the init process is ready but not started.
//   - [StateRunning]: after [Controller.Start] succeeds; the init process is executing.
//     Exec processes may be added via [Controller.NewProcess].
//   - [StateStopped]: terminal state reached when the init process exits;
//     all host-side resources have been released.
//   - [StateInvalid]: entered when [Controller.Create] or [Controller.Start] fails
//     mid-way; host-side resources are released. If the failure occurred after the
//     GCS container was successfully created, guest-side state may still require
//     cleanup via [Controller.DeleteProcess].
//
// # Resource Allocation
//
// During [Controller.Create], three categories of host-side resources are allocated
// and mapped into the guest:
//
//   - Layers: read-only image layers and the writable scratch layer are attached
//     via SCSI and combined inside the guest to form the container rootfs.
//   - Mounts: OCI spec mounts are dispatched by type — disk-backed mounts go through
//     SCSI, host-directory bind mounts go through Plan9 shares, and guest-internal
//     or unknown types pass through unmodified.
//   - Devices: Windows vPCI devices listed in the OCI spec are reserved on the host,
//     added to the VM, and their spec entries are rewritten to VMBus GUIDs.
//
// All allocated resources are released in reverse order during container teardown.
//
// # Process Management
//
// The init process (exec ID "") is created during [Controller.Create] and started
// during [Controller.Start]. Additional exec processes can be added to a running
// container via [Controller.NewProcess]. When the init process exits, the controller
// tears down all host-side resources and transitions to [StateStopped].
package linuxcontainer
