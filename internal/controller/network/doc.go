//go:build windows && (lcow || wcow)

// Package network provides a controller for managing the network lifecycle of a pod
// running inside a Utility VM (UVM).
//
// It handles attaching an HCN namespace and its endpoints to the guest VM,
// and tearing them down on pod removal.
//
// # Lifecycle
//
// A network follows the state machine below.
//
//	       ┌────────────────────┐
//	       │ StateNotConfigured │
//	       └───┬────────────┬───┘
//	Setup ok   │            │ Setup fails
//	           ▼            ▼
//	┌─────────────────┐  ┌──────────────┐
//	│ StateConfigured │  │ StateInvalid │
//	└────────┬────────┘  └──────┬───────┘
//	         │ Teardown         │ Teardown
//	         ▼                  ▼
//	┌─────────────────────────────────────┐
//	│           StateTornDown             │
//	└─────────────────────────────────────┘
//
// State descriptions:
//
//   - [StateNotConfigured]: initial state; no namespace or NICs have been configured.
//   - [StateConfigured]: after [Controller.Setup] succeeds; the HCN namespace is attached
//     and all endpoints are wired up inside the guest.
//   - [StateInvalid]: entered when [Controller.Setup] fails mid-way; best-effort
//     cleanup should be performed via [Controller.Teardown].
//   - [StateTornDown]: terminal state reached after [Controller.Teardown] completes.
//
// # Platform Variants
//
// Guest-side operations differ between LCOW and WCOW and are implemented in
// platform-specific source files selected via build tags
// ("lcow" tag for LCOW shim, "wcow" tag for WCOW shim).
package network
