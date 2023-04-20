# Package scsi

This README intends to act as internal developer guidance for the package. Guidance
to consumers of the package should be included as Go doc comments in the package code
itself.

## Terminology

We will generally use the term "attachment" to refer to a SCSI device being made
available to a VM, so that it is visible to the guest OS.
We will generally use the term "mount" to refer to a SCSI device being mounted
to a specific path, and with specific settings (e.g. encryption) inside
a guest OS.
Note that in `Manager`, "Mount" refers to the overall action of attaching and
mounting in the guest OS. If we come up with a better name for this aggregate
action, we should rename this.

## Principles

The general principle of this package is that attachments and mounts of SCSI devices
are completely separate operations, so they should be tracked and handled separately.
Out of a desire for convenience to the client of the package, we provide a `Manager`
type which handles them together, but under the hood attach/mount are managed by
separate components.

## Architecture

The package is composed of several layers of components:

### Top level, the `Manager` type

`Manager` is an exported type which acts as the primary entrypoint from external
consumers. It provides several methods which can be used to attach+mount a SCSI
device to a VM.

`Add*` methods on `Manager` return a `Mount`, which serves two purposes:
- Provides information to the caller on the attachment/mount, such as controller,
  LUN, and guest OS mount path.
- Tracks the resources associated with the SCSI attachment/mount, and provides a
  `Release` method to clean them up.

`Manager` itself is a fairly thin layer on top of two unexported types: `attachManager`
and `mountManager`.

### Mid level, the `attachManager` and `mountManager` types

These types are responsible for actually managing the state of attachments and mounts
on the VM.

`attachManager`:
- Tracks what SCSI devices are currently attached, along with what controllers/LUNs are
  used.
- When it is asked to attach a new SCSI device, it will first check if the attachment
  already exists, and increase its refcount if so. If not, it will allocate a new
  controller/LUN slot for the attachment, and then call the `attacher` to actually carry
  out the attach operation.
- When it is asked to detach a SCSI device, it first uses the `unplugger` to carry out any
  guest-side remove actions, and then uses the `attacher` to remove the attachment from
  the VM.
- Tracks refcount on any attached SCSI devices, so that they are not detached until there
  has been a detach request for each matching attach request.

`mountManager`:
- Tracks current SCSI devices mounted in the guest OS, and what mount options were applied.
- When it is asked to mount a new SCSI device, it will first check if the mount (with same options)
  already exists, and increase its refcount if so. If not, it will track the new mount, and
  call the `mounter` to actually carry out the guest mount operation.
- Tracks refcount on any mounted SCSI devices, so that they are not unmounted until there has
  been an unmount request for each matching mount request.

### Low level, the backend types

There are three interfaces that provide the low-level implementation to `attachManager` and
`mountManager`. They are `attacher`, `mounter` and `unplugger`.

- `attacher` provides the host-side operations of attach/detach of SCSI devices to a VM.
- `mounter` provides the guest-side operations of mount/unmount of SCSI devices inside the
  guest OS.
- `unplugger` provides the guest-side operation of safely removing a SCSI device, before it
  is detached from the VM.

To improve clarity, these three interfaces are grouped into the external interfaces `HostBackend`,
consisting of `attacher`, and `GuestBackend`, consisting of `mounter` and `unplugger`. There are
also corresponding implementations of the external interfaces for HCS and direct bridge connections.

The client passes in implementations of `HostBackend` and `GuestBackend` when instantiating the
`Manager`.

## Future work

Some thoughts on how this package could evolve in the future. This is intended to inform the direction
of future changes as we work in the package.

- The `mountManager` actually has very little to do with SCSI (at least for Linux containers, Windows
  may be more complicated/limited). In fact, the only SCSI-specific part of mounting in the guest is
  pretty much just identifying a block device based on the SCSI controller/LUN. It would be interesting
  to separate out the SCSI controller/LUN -> block device mapping part from the rest of the guest mount
  operation. This could enable us to e.g. use the same "mount" operation for SCSI and VPMEM, since they
  both present a block device.
- We should not be silently and automatically scanning a SCSI device for verity info. Determining what
  (if any) verity info to use for a device should probably be determined by the client of the package.
- Likewise, ACLing disks so a VM can access them should likely fall outside the purview of the package
  as well.
- The implemnentations of `HostBackend` and `GuestBackend` should probably live outisde
  the package. There is no real reason for them to be defined in here, other than not having a clear
  place to put them instead right now. They would make more sense to live close to the concrete
  implementation they depend upon. For instance, `bridgeGuestBackend` might make sense to be near the
  GCS bridge connection implementation.
- For unmounting, it is awkward to have to re-pass the mount configuration to the guest again. There is
  not a clear course of action if this differs from the original mount configuration, nor is this checked
  anywhere. It would be good for the guest to instead track what cleanup is needed for each mount point,
  and then we don't need to pass anything except the mount point in the unmount request.