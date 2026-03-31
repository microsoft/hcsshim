//go:build windows

// Package vpci provides a controller for managing virtual PCI (vPCI) device
// assignments on a Utility VM (UVM). It handles assigning and removing
// PCI devices from the UVM via HCS modify calls.
//
// # Lifecycle
//
// [Controller] tracks active device assignments by VMBus GUID (device identifier
// within UVM) in an internal map. Each assignment is reference-counted to
// support shared access by multiple callers.
//
//   - [Controller.Reserve] generates a unique VMBus GUID for a device and
//     records the reservation. If the same device is already reserved, the
//     existing GUID is returned.
//   - [Controller.AddToVM] assigns a previously reserved device to the VM
//     using the VMBus GUID returned by Reserve. If the device is already
//     assigned, the reference count is incremented and the call succeeds
//     without a second host-side assignment.
//   - [Controller.RemoveFromVM] decrements the reference count for the device
//     identified by VMBus GUID. When it reaches zero, the device is removed
//     from the VM. It also handles cleanup for devices that were reserved
//     but never assigned.
//
// # Invalid Devices
//
// If the host-side assignment succeeds but the guest-side notification fails,
// the device is marked invalid. It remains tracked so that the caller can call
// [Controller.RemoveFromVM] to perform host-side cleanup.
//
// # Virtual Functions
//
// Each Virtual Function is assigned as an independent guest device with its own
// VMBus GUID. Multiple Virtual Functions on the same physical device are treated
// as separate devices in the guest.
//
// # Guest Requests
//
// On LCOW, assigning a vPCI device requires a guest-side notification so the
// GCS can wait for the required device paths to become available.
// WCOW does not require a guest request as part of device assignment.
package vpci
