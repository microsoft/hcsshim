//go:build windows && (lcow || wcow)

// Package vm provides a controller for managing the lifecycle of a Utility VM (UVM).
//
// A Utility VM is a lightweight virtual machine used to host Linux (LCOW) or
// Windows (WCOW) containers. This package abstracts the VM lifecycle —
// creation, startup, stats collection, and termination — with the [Controller]
// as the primary implementation.
//
// Live-migration entry points are provided on both sides: the source captures a
// running VM via [Controller.Save] (state snapshot), while the destination
// rehydrates it via [Controller.Import] (state-only rehydration), recreates the
// VM, and rebinds its disks via [Controller.Patch] before resuming.
// [Controller.Resume] returns either side to [StateRunning].
//
// # Lifecycle
//
// A VM follows the state machine below.
//
//	      ┌─────────────────┐
//	      │ StateNotCreated │
//	      └────────┬────────┘
//	               │ CreateVM ok
//	               ▼
//	      ┌─────────────────┐           StartVM fails /
//	      │  StateCreated   │──────── TerminateVM fails ──────┐
//	      └──┬─────┬────────┘                                 │
//	         │     │ StartVM ok                               ▼
//	         │     ▼                                  ┌───────────────┐
//	         │  ┌─────────────────┐  TerminateVM      │  StateInvalid │
//	         │  │  StateRunning   │───── fails ──────►│               │
//	         │  └────────┬────────┘                   └───────┬───────┘
//	         │           │ VM exits /                         │ TerminateVM ok
//	TerminateVM ok       │ TerminateVM ok                     │
//	         │           ▼                                    ▼
//	         │  ┌─────────────────────────────────────────────────┐
//	         └─►│                 StateTerminated                 │
//	            └─────────────────────────────────────────────────┘
//
// Live migration walks a granular, side-specific path. Each side's synchronous memory
// transfer converges on the shared [StateMemoryTransferred], then a resume finalize
// reaches [StateMigrationFinalized] (from which [Controller.Resume] returns it to
// [StateRunning]); a stop finalize or [Controller.TerminateVM] reaches [StateTerminated].
//
//	source
//	┌─────────────────────────────────┐
//	│           StateRunning          │
//	└────────────────┬────────────────┘
//	                 │ InitializeLiveMigrationOnSource
//	                 ▼
//	┌─────────────────────────────────┐
//	│ StateSourceMigrationInitialized │  ── Save ──▶ (self)
//	└────────────────┬────────────────┘
//	                 │ StartLiveMigrationOnSource
//	                 ▼
//	┌─────────────────────────────────┐
//	│   StateSourceMigrationStarted   │
//	└────────────────┬────────────────┘
//	                 │ StartLiveMigrationTransfer
//	                 ▼
//	┌─────────────────────────────────┐
//	│      StateMemoryTransferred     │  ── Finalize(Stop) ──▶ StateTerminated
//	└────────────────┬────────────────┘
//	                 │ Finalize(Resume)
//	                 ▼
//	┌─────────────────────────────────┐
//	│     StateMigrationFinalized     │  ── Resume ──▶ StateRunning
//	└─────────────────────────────────┘
//
//	destination
//	┌───────────────────────────────────┐
//	│ StateDestinationMigrationImported │
//	└──────────────────┬────────────────┘
//	                   │ CreateVM
//	                   ▼
//	┌───────────────────────────────────┐
//	│  StateDestinationMigrationCreated │
//	└──────────────────┬────────────────┘
//	                   │ Patch
//	                   ▼
//	┌───────────────────────────────────┐
//	│  StateDestinationMigrationPatched │
//	└──────────────────┬────────────────┘
//	                   │ StartWithMigrationOptions
//	                   ▼
//	┌───────────────────────────────────┐
//	│  StateDestinationMigrationStarted │
//	└──────────────────┬────────────────┘
//	                   │ StartLiveMigrationTransfer
//	                   ▼
//	┌───────────────────────────────────┐
//	│       StateMemoryTransferred      │  ── Finalize(Stop) ──▶ StateTerminated
//	└──────────────────┬────────────────┘
//	                   │ Finalize(Resume)
//	                   ▼
//	┌───────────────────────────────────┐
//	│      StateMigrationFinalized      │  ── Resume ──▶ StateRunning
//	└───────────────────────────────────┘
//
// State descriptions:
//
//   - [StateNotCreated]: initial state after [New] is called.
//   - [StateCreated]: after [Controller.CreateVM] succeeds; the VM exists but has not started.
//   - [StateRunning]: after [Controller.StartVM] succeeds; the guest OS is up and the
//     Guest Compute Service (GCS) connection is established.
//   - [StateTerminated]: terminal state reached after the VM exits naturally or
//     [Controller.TerminateVM] completes successfully.
//   - [StateInvalid]: error state entered when [Controller.StartVM] fails after the underlying
//     HCS VM has already started, or when [Controller.TerminateVM] fails during uvm.Close.
//     A VM in this state can only be cleaned up by calling [Controller.TerminateVM].
//   - [StateSourceMigrationInitialized]: the running source VM has begun an outgoing migration
//     via [Controller.InitializeLiveMigrationOnSource]; only [Controller.Save] and live-migration
//     calls are permitted.
//   - [StateSourceMigrationStarted]: the source is streaming state via [Controller.StartLiveMigrationOnSource].
//   - [StateDestinationMigrationImported]: the destination has been rehydrated from a snapshot via
//     [Controller.Import] but the VM does not exist yet; [Controller.CreateVM] is the next step.
//   - [StateDestinationMigrationCreated]: the destination VM has been created from the snapshot but
//     not started; [Controller.Patch] rebinds its disks.
//   - [StateDestinationMigrationPatched]: the destination VM's disks have been rebound; [Controller.StartWithMigrationOptions] is next.
//   - [StateDestinationMigrationStarted]: the destination VM is running against the migration
//     transport awaiting the source's state.
//   - [StateMemoryTransferred]: the synchronous memory transfer has completed on either side via
//     [Controller.StartLiveMigrationTransfer]; [Controller.FinalizeLiveMigration] is next.
//   - [StateMigrationFinalized]: a resume finalize has completed on either side; [Controller.Resume]
//     returns it to [StateRunning].
//
// # Platform Variants
//
// Certain behaviors differ between LCOW and WCOW guests and are implemented in
// platform-specific source files selected via build tags (default for lcow shim and "wcow" tag for wcow shim).
//
// # Usage
//
//	ctrl := vm.New()
//
//	if err := ctrl.CreateVM(ctx, &vm.CreateOptions{
//	    ID:          "my-uvm",
//	    Owner:       "my-shim",
//	    BundlePath:  bundlePath,
//	    ShimOpts:    shimOpts,
//	    SandboxSpec: sandboxSpec,
//	}); err != nil {
//	    // handle error
//	}
//
//	if err := ctrl.StartVM(ctx, &vm.StartOptions{
//	    GCSServiceID: serviceGUID,
//	}); err != nil {
//	    // handle error
//	}
//
//	// ... use ctrl.Guest() for guest interactions ...
//
//	if err := ctrl.TerminateVM(ctx); err != nil {
//	    // handle error
//	}
//
//	_ = ctrl.Wait(ctx)
package vm
