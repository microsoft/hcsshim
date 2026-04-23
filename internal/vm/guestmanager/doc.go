//go:build windows && (lcow || wcow)

/*
Package guestmanager manages guest-side operations for utility VMs (UVMs) via the
GCS (Guest Compute Service) connection.

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

	_ = g.AddNetworkInterface(ctx, &guestresource.LCOWNetworkAdapter{...})
	_ = g.AddMappedVirtualDisk(ctx, guestresource.LCOWMappedVirtualDisk{...})

# Layer Boundaries

This package covers guest-side changes executed over the GCS connection. Host-side
VM configuration and lifecycle operations belong in the sibling vmmanager package.
*/
package guestmanager
