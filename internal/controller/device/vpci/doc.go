//go:build windows

// Package vpci provides a controller for managing virtual PCI (vPCI) device
// assignments on a Utility VM (UVM). It handles assigning and removing
// PCI devices from the UVM via HCS modify calls.
//
// # Lifecycle
//
// [Manager] tracks active device assignments by VMBus GUID (device identifier
// within UVM) in an internal map. Each assignment is reference-counted to
// support shared access by multiple callers.
//
//   - [Controller.AddToVM] assigns a device and records it in the map.
//     If the same device is already assigned, the reference count is incremented.
//   - [Controller.RemoveFromVM] decrements the reference count for the device
//     identified by VMBus GUID. When it reaches zero, the device is removed
//     from the VM.
//
// # Invalid Devices
//
// If the host-side assignment succeeds but the guest-side notification fails,
// the device is marked invalid. It remains tracked so that the caller can call
// [Controller.RemoveFromVM] to perform host-side cleanup.
//
// # Guest Requests
//
// On LCOW, assigning a vPCI device requires a guest-side notification so the
// GCS can wait for the required device paths to become available.
// WCOW does not require a guest request as part of device assignment.
package vpci
