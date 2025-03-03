## Overview
Applications connecting from the host into the container should use container-specific VMID.
This VMID will need to be the same as the container's VMID inside the guest. One way to get
the VMID is to query HCS for it or use this binary, which outputs the same VMID, when
querying HCS isn't an option.

## Build
Build the binary as following
```powershell
> go build ./internal/tools/hvsocketaddr
```

## Run
Find container ID using (e.g.) `crictl.exe`:
```powershell
> crictl ps --no-trunc
```
Note that we need full container ID, rather than a truncated one.

Get VMID:
```powershell
> .\hvsocketaddr.exe <container-id>
```
The output VMID can be used by the services on the host.