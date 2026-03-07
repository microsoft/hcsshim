//go:build windows

// Package vm provides a controller for managing the lifecycle of a Utility VM (UVM).
//
// A Utility VM is a lightweight virtual machine used to host Linux (LCOW) or
// Windows (WCOW) containers. This package abstracts the VM lifecycle —
// creation, startup, stats collection, and termination — behind the [Controller]
// interface, with [Manager] as the primary implementation.
//
// # Lifecycle
//
// A VM progresses through the following states:
//
//		[StateNotCreated] → [StateCreated] → [StateRunning] → [StateStopped]
//
//	  - [StateNotCreated]: initial state after [NewController] is called.
//	  - [StateCreated]: after [Controller.CreateVM] succeeds; the VM process exists but has not started.
//	  - [StateRunning]: after [Controller.StartVM] succeeds; the guest OS is up and the
//	    Guest Compute Service (GCS) connection is established.
//	  - [StateStopped]: terminal state reached after the VM exits or [Controller.TerminateVM] is called.
//
// # Platform Variants
//
// Certain behaviours differ between LCOW and WCOW guests and are implemented in
// platform-specific source files selected via build tags (default for lcow shim and "wcow" tag for wcow shim).
//
// # Usage
//
//	ctrl := vm.NewController()
//
//	if err := ctrl.CreateVM(ctx, &vm.CreateOptions{
//	    ID:          "my-uvm",
//	    HCSDocument: doc,
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
