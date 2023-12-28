# UVM Boot Info

For understanding the UVM's boot sequence it's useful to think of the UVM as consisting of:
- Linux kernel
- Kernel command line
  - The command line is a set of parameters the kernel understands which correspond to actions it will perform during boot.
- Root filesystem (rootfs) disk
  - This contains all the files that exist when first starting the VM.
- Startup script
  - Stored in the rootfs disk. This scripts does the last bits of setup required to get the VM ready for use.
- Hash disk (SNP Mode only)
  - Containing DM-Verity hash data (read more below about DM-Verity and SNP mode below).


## The SNP Mode UVM boot sequence.
- The vmgs (kernel + commandline) file is loaded into memory.
- The instructions from the kernel command line are performed, the kernel:
    - Checks the hash disk's hash data (a merkle tree) is consistent.
    - Checks the hash disk's root hash matches the root hash in the kernel command line. The boot fails if not because the integrity of the UVM cannot be confirmed.
    - Makes the rootfs disk available as a dm-verity device.
    - Mounts the dm-verity rootfs device.
    - Sets the newly mounted disk as the root filesystem
    - Finds and runs the startup script (which is specified in the kernel command line) from the rootfs to initialise the system.
    - Anytime that data is read from the dm-verity rootfs, that data's integrity is checked on the fly by comparing the data's hash with the hash data on the hash disk.
