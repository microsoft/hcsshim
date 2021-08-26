# Rootfs Tool

Take a container image name and tag and a list(slice) of rootfs hashes for each layer.

`rootfs` exists as a tool to make it easier to calculate image rootfs hash for developers
 working functionality related to image rootfs or security policy in this repository.
It is not intended to be used by "end users" but could be used as a basis for
such a tool.

Running the tool can take a long time as each layer for each container must
be downloaded, turned into an ext4, and finally a dm-verity root hash calculated.

## Example Command to run the tool

```bash
# run towards remote container registry
go run main.go -i rust -t 1.52.1 -d remote
# run towards local container daemon/repository
go run main.go -i rust -t 1.52.1 -d local
```

### Example output

The above command gets translated into the appropriate rootfs hashes for each layer.

```bash
[0]: fe84c9d5bfddd07a2624d00333cf13c1a9c941f3a261f13ead44fc6a93bc0e7a
[1]: 4dedae42847c704da891a28c25d32201a1ae440bce2aecccfa8e6f03b97a6a6c
[2]: 41d64cdeb347bf236b4c13b7403b633ff11f1cf94dbc7cf881a44d6da88c5156
[3]: eb36921e1f82af46dfe248ef8f1b3afb6a5230a64181d960d10237a08cd73c79
[4]: e769d7487cc314d3ee748a4440805317c19262c7acd2fdbdb0d47d2e4613a15c
[5]: 1b80f120dbd88e4355d6241b519c3e25290215c469516b49dece9cf07175a766

```

## CLI Options

- -i (required)

Image name to process

- -t (optional)

Image tag to process. If not specified, will use the default value `latest`.

- -d (optional)

Container registry destination, the value should fall in following values: [`local`, `remote`]. If not specified, will use the default value `local`.
