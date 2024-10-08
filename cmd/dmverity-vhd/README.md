# dmverity-vhd

Takes an OCI image locator and an output directory and converts the layers that
make up the image into a series of VHDs in the output directory. One VHD will
be created per image layer.

VHDs are named with the name of the layer SHA.

Each layer contains
[dm-verity](https://www.kernel.org/doc/html/latest/admin-guide/device-mapper/verity.html)
information that can be used to ensure the integrity of the created ext4
filesystem. All VHDs have a layout of:

- ext4 filesystem
- dm-verity superblock
- dm-verity merkle tree
- VHD footer

The output is deterministic except for the UUIDs embedded in the VHD footer and
the dm-verity superblock. Both UUIDs are currently seeded using a random number
generator.

## Example usage

Create VHDs:

```bash
dmverity-vhd create -i alpine:3.12 -o alpine_3_12_layers
```

Output:

```text
Layer VHD created at alpine_3_12_layers\1ad27bdd166b922492031b1938a4fb2f775e3d98c8f1b72051dad0570a4dd1b5.vhd
```

Create VHDs from a directory tarball:

```bash
dmverity-vhd create -i data.tar -o data_vhd -dir
```

Output:

```text
Directory VHD created at data_vhd\data.vhd
```

Compute root hashes:

```bash
dmverity-vhd --docker roothash -i alpine:3.12
```

Output:

```text
Layer 0 root hash: 71702a459fa5e6574337e014d9d3936bcf7cb448aaffe3814883caa01fbb4827
```

Compute root hashes with tarball:

```bash
dmverity-vhd --tarball /path/to/tarball.tar roothash -i alpine:3.12
```
