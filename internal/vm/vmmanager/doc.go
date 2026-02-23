//go:build windows

/*
Package vmmanager manages host-side VM configuration and lifecycle for utility VMs (UVMs).

It provides a concrete [UtilityVM] struct and a set of granular manager interfaces, each
scoped to an individual resource concern:

  - [LifetimeManager] – start, terminate, close, pause, resume, save, and wait.
  - [NetworkManager] – add, remove, and update NICs.
  - [SCSIManager] – hot-add and remove SCSI disks.
  - [PCIManager] – assign and remove PCI devices.
  - [PipeManager] – add and remove named pipes.
  - [Plan9Manager] – add and remove Plan 9 shares.
  - [VMSocketManager] – update and remove HvSocket services.
  - [VPMemManager] – add and remove virtual persistent memory devices.
  - [VSMBManager] – add and remove virtual SMB shares.
  - [ResourceManager] – CPU group, CPU limits, and memory updates.

All interfaces are implemented by [UtilityVM].

Presently this package is tightly coupled with the HCS backend and only runs
HCS-backed UVMs. It does not store host or guest side state; that is the
responsibility of the orchestration layer above it.

# Creating a UVM

Build an [hcsschema.ComputeSystem] configuration, then call [Create]:

	config := &hcsschema.ComputeSystem{ ... }
	uvm, err := vmmanager.Create(ctx, "uvm-id", config)
	if err != nil { // handle error }
	if err := uvm.Start(ctx); err != nil { // handle error }

After the VM is running, use the manager interfaces for host-side changes:

	_ = uvm.AddNIC(ctx, nicID, settings)

# Layer Boundaries

This package covers host-side changes and lifecycle operations on an existing
UVM. Guest-side actions (for example, mounting a disk) belong in the sibling
guestmanager package.
*/
package vmmanager
