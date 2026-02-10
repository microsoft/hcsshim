//go:build windows

/*
Package guestmanager manages guest-side operations for utility VMs (UVMs) via the
GCS (Guest Compute Service) connection.

It provides a concrete [Guest] struct, a top-level [Manager] interface that
aggregates connection lifecycle and container/process operations, and a set of
granular resource-scoped manager interfaces:

  - [Manager] – connection lifecycle, container and process creation, stack dumps,
    and container state deletion.
  - [LCOWNetworkManager] – add and remove network interfaces in an LCOW guest.
  - [WCOWNetworkManager] – add and remove network interfaces and namespaces in a WCOW guest.
  - [LCOWDirectoryManager] – map and unmap directories in an LCOW guest.
  - [WCOWDirectoryManager] – map directories in a WCOW guest.
  - [LCOWScsiManager] – add and remove mapped virtual disks and SCSI devices in an LCOW guest.
  - [WCOWScsiManager] – add and remove mapped virtual disks and SCSI devices in a WCOW guest.
  - [LCOWLayersManager] – add and remove combined layers in an LCOW guest.
  - [WCOWLayersManager] – add and remove combined layers in a WCOW guest.
  - [CIMsManager] – add and remove WCOW block CIM mounts.
  - [LCOWDeviceManager] – add and remove VPCI and VPMem devices in an LCOW guest.
  - [SecurityPolicyManager] – add security policies and inject policy fragments.

All interfaces are implemented by [Guest].

This package is strictly guest-side. It does not own or modify host-side UVM
state; that is the responsibility of the sibling vmmanager package. It also does
not store UVM host or guest state — state management belongs to the orchestration
layer above.

# Creating a Guest

After the UVM has been started via vmmanager, create a [Guest] and establish the
GCS connection:

	g, err := guestmanager.New(ctx, uvm)
	if err != nil { // handle error }
	if err := g.CreateConnection(ctx); err != nil { // handle error }

After the connection is established, use the manager interfaces for guest-side changes:

	_ = g.AddLCOWNetworkInterface(ctx, &guestresource.LCOWNetworkAdapter{...})
	_ = g.AddLCOWMappedVirtualDisk(ctx, guestresource.LCOWMappedVirtualDisk{...})

# Layer Boundaries

This package covers guest-side changes executed over the GCS connection. Host-side
VM configuration and lifecycle operations belong in the sibling vmmanager package.
*/
package guestmanager
