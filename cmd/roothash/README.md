# roothash

Takes an OCI image locator and outputs a
[dm-verity](https://www.kernel.org/doc/html/latest/admin-guide/device-mapper/verity.html)
roothash for the each layer's filesystem.

## Example usage

```bash
roothash -i alpine:3.12
```
