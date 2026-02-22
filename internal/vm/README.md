# VM Package

This directory defines the utility VM (UVM) contracts and separates responsibilities into two layers. The goal is to keep
host-side management and guest-side actions distinct so each layer can evolve independently.

1. **VM Manager**: manages host-side VM configuration and lifecycle (NICs, SCSI, VPMem, etc.).
2. **Guest Manager**: intended for guest-side actions (for example, mounting a disk).

**Note that** this layer does not store UVM host or guest side state. That will be part of the orchestration layer above it.

## Packages and Responsibilities

- `internal/vm/vmmanager`
  - Concrete `UtilityVM` struct for running and managing a UVM instance.
  - Defines granular manager interfaces scoped to individual resource concerns (`LifetimeManager`, `NetworkManager`, `SCSIManager`, `PCIManager`, `PipeManager`, `Plan9Manager`, `VMSocketManager`, `VPMemManager`, `VSMBManager`, `ResourceManager`), all implemented by `UtilityVM`.
  - Presently, this package is tightly coupled with HCS backend and only runs HCS backed UVMs.
  - Owns lifecycle calls (start/terminate/close) and host-side modifications (NICs, SCSI, VPMem, pipes, VSMB, Plan9).
  - Allows creation of the UVM using `vmmanager.Create` which takes an `hcsschema.ComputeSystem` config and produces a running UVM.
- `internal/vm/guestmanager`
  - Reserved for guest-level actions such as mounting disks or performing in-guest configuration.
  - Currently empty and intended to grow as guest actions are formalized.

## Typical Flow

1. Build the `hcsschema.ComputeSystem` configuration.
2. Create the VM using the VM manager.
3. Use manager interfaces for lifecycle and host-side changes.
4. Use guest manager interfaces for in-guest actions (when available).

## Example (High Level)

```
config := &hcsschema.ComputeSystem{
	// Configure the VM document.
}

// Create and start the VM.
uvm, _ := vmmanager.Create(ctx, "uvm-id", config)
_ = uvm.Start(ctx)

// Apply host-side updates.
_ = uvm.AddNIC(ctx, nicID, settings)
```

## Layer Boundaries (Quick Reference)

- **VM Manager**: host-side changes and lifecycle operations on an existing UVM.
- **Guest Manager**: guest-side actions, scoped to work that requires in-guest context.

