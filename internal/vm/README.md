# VM Package

This directory defines the utility VM (UVM) contracts and separates responsibilities into three layers. The goal is to keep
configuration, host-side management, and guest-side actions distinct so each layer can evolve independently.

1. **Builder**: constructs an HCS compute system configuration used to create a VM.
2. **VM Manager**: manages host-side VM configuration and lifecycle (NICs, SCSI, VPMem, etc.).
3. **Guest Manager**: intended for guest-side actions (for example, mounting a disk).

**Note that** this layer does not store UVM host or guest side state. That will be part of the orchestration layer above it.

## Packages and Responsibilities

- `internal/vm/builder`
  - Interface definitions for shaping the VM configuration (`Builder` interface).
  - Concrete implementation of `Builder` for building `hcsschema.ComputeSystem` documents.
  - Provides a fluent API for configuring all aspects of the VM document.
  - Presently, this package is tightly coupled with HCS backend. 
- `internal/vm/vmmanager`
  - Interface definitions for UVM lifecycle and host-side management.
  - Concrete implementation of `UVM` for running and managing a UVM instance.
  - Presently, this package is tightly coupled with HCS backend and only runs HCS backed UVMs.
  - Owns lifecycle calls (start/terminate/close) and host-side modifications (NICs, SCSI, VPMem, pipes, VSMB, Plan9).
  - Allows creation of the UVM using `vmmanager.Create` which takes a `Builder` and produces a running UVM.
- `internal/vm/guestmanager`
  - Reserved for guest-level actions such as mounting disks or performing in-guest configuration.
  - Currently empty and intended to grow as guest actions are formalized.

## Typical Flow

1. Build the config using the builder interfaces.
2. Create the VM using the VM manager.
3. Use manager interfaces for lifecycle and host-side changes.
4. Use guest manager interfaces for in-guest actions (when available).

## Example (High Level)

```
builder, _ := builder.New("owner")

// Configure the VM document.
builder.SetMemory(&hcsschema.VirtualMachineMemory{SizeInMB: 1024})
builder.SetProcessor(&hcsschema.VirtualMachineProcessor{Count: 2})
// ... other builder configuration

// Create and start the VM.
uvm, _ := vmmanager.Create(ctx, "uvm-id", builder)
_ = uvm.LifetimeManager().Start(ctx)

// Apply host-side updates.
_ = uvm.NetworkManager().AddNIC(ctx, nicID, endpointID, macAddr)
```

## Layer Boundaries (Quick Reference)

- **Builder**: static, pre-create configuration only. No host mutations.
- **VM Manager**: host-side changes and lifecycle operations on an existing UVM.
- **Guest Manager**: guest-side actions, scoped to work that requires in-guest context.

