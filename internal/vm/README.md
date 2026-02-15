# VM Package

This directory defines the utility VM (UVM) contracts and separates responsibilities into three layers. The goal is to keep
configuration, host-side management, and guest-side actions distinct so each layer can evolve independently.

1. **Builder**: constructs an HCS compute system configuration used to create a VM.
2. **VM Manager**: manages host-side VM configuration and lifecycle (NICs, SCSI, VPMem, etc.).
3. **Guest Manager**: guest-side actions executed via the GCS connection (for example, mounting a mapped disk).

**Note that** this layer does not store UVM host or guest side state. That will be part of the orchestration layer above it.

## Packages and Responsibilities

- `internal/vm`
  - Shared types used across layers (currently `GuestOS`).
- `internal/vm/builder`
  - Interface definitions for shaping the VM configuration (`BootOptions`, `MemoryOptions`, `ProcessorOptions`, `DeviceOptions`,
    `NumaOptions`, `StorageQoSOptions`).
  - Concrete implementation of `UtilityVMBuilder` for building `hcsschema.ComputeSystem` documents (`UtilityVM`).
  - Provies a fluent API for constructing the `hcsschema.ComputeSystem` document used to create a UVM.
  - Presently, this package is tightly coupled with the HCS backend.
- `internal/vm/vmmanager`
  - Interface definitions for UVM lifecycle and host-side management.
  - Concrete implementation of `UtilityVM` for running and managing a UVM instance created from a builder document.
  - Owns lifecycle calls (start/terminate/close/pause/resume/save/wait) and host-side modifications:
    - Network adapters (`NetworkManager`), SCSI disks (`SCSIManager`), VPMem (`VPMemManager`), VSMB (`VSMBManager`), Plan9 (`Plan9Manager`).
    - Named pipes (`PipeManager`), virtual PCI devices (`PCIManager`), HvSocket services (`VMSocketManager`).
    - Resource updates such as CPU group/limits and memory (`ResourceManager`).
  - Presently, this package is tightly coupled with the HCS backend and only runs HCS-backed UVMs.
- `internal/vm/guestmanager`
  - Interface definitions for guest-side operations executed via the GCS connection.
  - Manages GCS connection lifecycle, including HvSocket setup and initial guest state.
  - Implements operations for guest resources, split by LCOW/WCOW where needed:
    - Network interfaces/namespaces, mapped directories, mapped virtual disks, combined layers, block CIMs (WCOW),
      VPCI/VPMem devices (LCOW), and security policy operations.
  - Translates guest operations into GCS modify requests.

## Typical Flow

1. Build the config using the builder options.
2. Create the VM using the VM manager.
3. Use manager methods for lifecycle and host-side changes.
4. Establish a GCS connection and use guest manager interfaces for in-guest actions.

## Example (High Level)

```
b, _ := builder.New("owner", vm.Linux)

// Parse into the correct builder option.
memoryOpts := builder.MemoryOptions(b)
processorOpts := builder.ProcessorOptions(b)

// Configure the VM document.
memoryOpts.SetMemoryLimit(1024)
processorOpts.SetProcessorLimits(&hcsschema.VirtualMachineProcessor{Count: 2})
// ... other builder configuration

// Create the VM.
uvm, _ := vmmanager.Create(ctx, "uvm-id", b)
_ = uvm.Start(ctx)

// Create the Guest Connection.
g, _ := guestmanager.New(ctx, uvm)

// Start the UVM.
lifetime := vmmanager.LifetimeManager(uvm)
_ = lifetime.Start(ctx)

// Create the connection.
guest := guestmanager.Manager(g)
_ = guest.CreateConnection(ctx)

// Apply host-side updates.
network := vmmanager.NetworkManager(uvm)
_ = network.AddNIC(ctx, nicID, &hcsschema.NetworkAdapter{})

// Apply guest-side updates.
guestNetwork := guestmanager.LCOWNetworkManager(g)
_ = guestNetwork.AddLCOWNetworkInterface(ctx, &guestresource.LCOWNetworkAdapter{})
```

## Layer Boundaries (Quick Reference)

- **Builder**: static, pre-create configuration only. No host mutations.
- **VM Manager**: host-side changes and lifecycle operations on an existing UVM.
- **Guest Manager**: guest-side actions, scoped to work that requires in-guest context (GCS-backed).
