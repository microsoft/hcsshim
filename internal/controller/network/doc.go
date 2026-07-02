//go:build windows && (lcow || wcow)

// Package network provides a controller for managing the network lifecycle of a pod
// running inside a Utility VM (UVM).
//
// It handles attaching an HCN namespace and its endpoints to the guest VM,
// and tearing them down on pod removal. Live-migration entry points are
// provided on both sides: the source freezes a configured network via
// [Controller.Save], while the destination rehydrates it via [Import]
// (state-only rehydration) and [Controller.Resume] (binds host/guest interfaces
// once the destination VM is running).
//
// # Lifecycle
//
// A network controller created via [New] follows the live-creation path:
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
// Live migration adds two branches. The destination rehydrates a controller via
// [Import] into [StateDestinationMigrating]; the source freezes a configured
// network via [Controller.Save] into [StateSourceMigrating]. Either returns to
// [StateConfigured] via [Controller.Resume], or to [StateTornDown] via
// [Controller.Teardown]:
//
//	┌───────────────────────────┐               ┌─────────────────┐
//	│ StateDestinationMigrating │── Resume ──▶  │ StateConfigured │
//	│            or             │               └─────────────────┘
//	│    StateSourceMigrating   │               ┌─────────────────┐
//	└───────────────────────────┘── Teardown ─▶ │  StateTornDown  │
//	                                            └─────────────────┘
//
// State descriptions:
//
//   - [StateNotConfigured]: initial state; no namespace or NICs have been configured.
//   - [StateConfigured]: after [Controller.Setup] succeeds; the HCN namespace is attached
//     and all endpoints are wired up inside the guest.
//   - [StateInvalid]: entered when [Controller.Setup] fails mid-way; best-effort
//     cleanup should be performed via [Controller.Teardown].
//   - [StateTornDown]: terminal state reached after [Controller.Teardown]
//     completes.
//   - [StateDestinationMigrating]: initial state for [Import] on the destination;
//     host/guest interfaces are not yet bound. [Controller.Resume] binds them and
//     moves to [StateConfigured]; [Controller.Teardown] aborts to [StateTornDown].
//   - [StateSourceMigrating]: a configured network frozen by [Controller.Save] on the
//     source while a migration is in flight. [Controller.Resume] rolls it back to
//     [StateConfigured]; [Controller.Teardown] tears it down to [StateTornDown].
//
// # Platform Variants
//
// Guest-side operations differ between LCOW and WCOW and are implemented in
// platform-specific source files selected via build tags
// ("lcow" tag for LCOW shim, "wcow" tag for WCOW shim).
package network
