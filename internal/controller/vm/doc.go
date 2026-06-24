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
//		         ┌─────────────────┐
//		         │ StateNotCreated │
//		         └────────┬────────┘
//		                  │ CreateVM ok
//		                  ▼
//		         ┌─────────────────┐           StartVM fails /
//		         │  StateCreated   │──────── TerminateVM fails ──────┐
//		         └──┬─────┬────────┘                                 │
//		            │     │ StartVM ok                               ▼
//		            │     ▼                                  ┌───────────────┐
//		            │  ┌─────────────────┐  TerminateVM      │  StateInvalid │
//		            │  │  StateRunning   │───── fails ──────►│               │
//		            │  └────────┬────────┘                   └───────┬───────┘
//		            │           │ VM exits /                         │ TerminateVM ok
//	      TerminateVM ok        │ TerminateVM ok                     │
//		            │           ▼                                    ▼
//		            │  ┌─────────────────────────────────────────────────┐
//		            └─►│                 StateTerminated                 │
//		               └─────────────────────────────────────────────────┘
//
// Live migration adds side-specific paths. The source toggles a running VM into
// [StateSourceMigrating]; the destination walks a dedicated path —
// [Controller.Import] → [Controller.CreateVM] → [Controller.StartWithMigrationOptions].
// From either migrating state the resumed side returns to [StateRunning] via
// [Controller.Resume], while the stopped side reaches [StateTerminated] via a
// finalize Stop or a teardown ([Controller.TerminateVM]). The forward flow stops
// the source and resumes the destination; the reverse flow resumes the source and
// stops the destination.
//
//	      source                                  destination
//	┌──────────────────────┐         ┌───────────────────────────┐
//	│ StateSourceMigrating │         │  StateMigratingImported   │
//	└───┬──────────────┬───┘         └─────────────┬─────────────┘
//	    │ Resume       │ Finalize(Stop)/Terminate  │ CreateVM
//	    ▼              ▼                           ▼
//	StateRunning  StateTerminated     ┌───────────────────────────┐
//	                                  │   StateMigratingCreated   │
//	                                  └─────────────┬─────────────┘
//	                                                │ StartWithMigrationOptions
//	                                                ▼
//	                                  ┌───────────────────────────┐
//	                                  │ StateDestinationMigrating │
//	                                  └──────┬─────────────┬──────┘
//	                                  Resume │             │ Finalize(Stop)/Terminate
//	                                         ▼             ▼
//	                                   StateRunning   StateTerminated
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
//     HCS VM has already started, or when [Controller.TerminateVM] fails during uvm.Close
//     (from [StateCreated], [StateRunning], or [StateMigratingCreated]).
//     A VM in this state can only be cleaned up by calling [Controller.TerminateVM].
//   - [StateSourceMigrating]: the running source VM has begun an outgoing migration;
//     only live-migration calls and [Controller.Save] are permitted. [Controller.Resume]
//     rolls it back to [StateRunning]; a finalize Stop (forward flow) or
//     [Controller.TerminateVM] terminates it to [StateTerminated].
//   - [StateMigratingImported]: the destination has been rehydrated from a snapshot via
//     [Controller.Import] but the VM does not exist yet; [Controller.CreateVM] is the next step.
//   - [StateMigratingCreated]: the destination VM has been created from the snapshot but not
//     started; disks are rebound via [Controller.Patch] and
//     [Controller.StartWithMigrationOptions] advances it to [StateDestinationMigrating].
//   - [StateDestinationMigrating]: the destination VM is running against the migration
//     transport awaiting the source's state; [Controller.Resume] reaches [StateRunning],
//     while a finalize Stop (reverse flow) or [Controller.TerminateVM] terminates it to
//     [StateTerminated].
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
