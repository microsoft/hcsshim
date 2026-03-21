//go:build windows && !wcow

// Package plan9 provides a controller for managing Plan9 file-share devices
// attached to a Utility VM (UVM).
//
// It handles attaching and detaching Plan9 shares to the VM via HCS modify
// calls. Guest-side mount operations (mapped-directory requests) are handled
// separately by the mount controller.
//
// The [Controller] interface is the primary entry point, with [Manager] as its
// concrete implementation. A single [Manager] manages all Plan9 shares for a UVM.
//
// # Lifecycle
//
// [Manager] tracks active shares by name in an internal map.
//
//   - [Controller.AddToVM] adds a share and records its name in the map.
//     If the HCS call fails, the share is not recorded.
//   - [Controller.RemoveFromVM] removes a share and deletes its name from the map.
//     If the share is not in the map, the call is a no-op.
package plan9
