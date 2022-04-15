# securitypolicy

Takes a configuration to a TOML file and outputs a Base64 encoded string of the
generated security policy.

`securitypolicy` exists as a tool to make it easier to generate security policies
for developers working functionality related to security policy in this repository.
It is not intended to be used by "end users" but could be used as a basis for
such a tool.

A Base64 encoded version of policy is sent as an annotation to GCS for processing.
The `securitypolicy` tool will, by default, output Base64 encoded JSON.

Running the tool can take a long time as each layer for each container must
be downloaded, turned into an ext4, and finally a dm-verity root hash calculated.

## Example TOML configuration file

```toml
[[container]]
image_name = "rust:1.52.1"
command = ["rustc", "--help"]
working_dir = "/home/user"
expected_mounts = ["/path/to/container/mount-1", "/path/to/container/mount-2"]

[[container.env_rule]]
strategy = "re2"
rule = "PREFIX_.+=.+"

[[container.mount]]
host_path = "sandbox://host/path/one"
container_path = "/container/path/one"
readonly = false

[[container.mount]]
host_path = "sandbox://host/path/two"
container_path = "/container/path/two"
readonly = true
```

### Converted to JSON

The above TOML configuration gets translated into the appropriate policy that is
represented in JSON.

```json
{
  "allow_all": false,
  "containers": {
    "length": 2,
    "elements": {
      "0": {
        "command": {
          "length": 2,
          "elements": {
            "0": "rustc",
            "1": "--help"
          }
        },
        "env_rules": {
          "length": 6,
          "elements": {
            "0": {
              "strategy": "string",
              "rule": "PATH=/usr/local/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
            },
            "1": {
              "strategy": "string",
              "rule": "RUSTUP_HOME=/usr/local/rustup"
            },
            "2": {
              "strategy": "string",
              "rule": "CARGO_HOME=/usr/local/cargo"
            },
            "3": {
              "strategy": "string",
              "rule": "RUST_VERSION=1.52.1"
            },
            "4": {
              "strategy": "string",
              "rule": "TERM=xterm"
            },
            "5": {
              "strategy": "re2",
              "rule": "PREFIX_.+=.+"
            }
          }
        },
        "layers": {
          "length": 6,
          "elements": {
            "0": "fe84c9d5bfddd07a2624d00333cf13c1a9c941f3a261f13ead44fc6a93bc0e7a",
            "1": "4dedae42847c704da891a28c25d32201a1ae440bce2aecccfa8e6f03b97a6a6c",
            "2": "41d64cdeb347bf236b4c13b7403b633ff11f1cf94dbc7cf881a44d6da88c5156",
            "3": "eb36921e1f82af46dfe248ef8f1b3afb6a5230a64181d960d10237a08cd73c79",
            "4": "e769d7487cc314d3ee748a4440805317c19262c7acd2fdbdb0d47d2e4613a15c",
            "5": "1b80f120dbd88e4355d6241b519c3e25290215c469516b49dece9cf07175a766"
          }
        },
        "working_dir": "/home/user",
        "expected_mounts": {
          "length": 2,
          "elements": {
            "0": "/path/to/container/mount-1",
            "1": "/path/to/container/mount-2"
          }
        },
        "mounts": {
          "length": 2,
          "elements": {
            "0": {
              "source": "sandbox://host/path/one",
              "destination": "/container/path/one",
              "type": "bind",
              "options": {
                "length": 3,
                "elements": {
                  "0": "rbind",
                  "1": "rprivate",
                  "2": "rw"
                }
              }
            },
            "1": {
              "source": "sandbox://host/path/two",
              "destination": "/container/path/two",
              "type": "bind",
              "options": {
                "length": 3,
                "elements": {
                  "0": "rbind",
                  "1": "rprivate",
                  "2": "ro"
                }
              }
            }
          }
        }
      },
      "1": {
        "command": {
          "length": 1,
          "elements": {
            "0": "/pause"
          }
        },
        "env_rules": {
          "length": 2,
          "elements": {
            "0": {
              "strategy": "string",
              "rule": "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
            },
            "1": {
              "strategy": "string",
              "rule": "TERM=xterm"
            }
          }
        },
        "layers": {
          "length": 1,
          "elements": {
            "0": "16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"
          }
        },
        "working_dir": "/",
        "expected_mounts": {
          "length": 0,
          "elements": {}
        },
        "mounts": {
          "length": 0,
          "elements": {}
        }
      }
    }
  }
}
```

## CLI Options

- -c

TOML configuration file to process (required)

- -j

output raw JSON in addition to the Base64 encoded version

## Authorization

Some images will be pulled from registries that require authorization. To add
authorization information for a given image, you would add an `[auth]` object
to the TOML definition for that image. For example:

```toml
[[container]]
image_name = "rust:1.52.1"
command = ["rustc", "--help"]

[auth]
username = "my username"
password = "my password"
```

Authorization information needs to be added on a per-image basis as it can vary
from image to image and their respective registries.

To pull an image using anonymous access, no `[auth]` object is required.

## Pause container

All LCOW pods require a pause container to run. The pause container must be
included in the policy. As this tool is aimed at LCOW developers, a default
version of the pause container is automatically added to policy even though it
isn't in the TOML configuration.

If the version of the pause container changes from 3.1, you will need to update
the hardcoded root hash by running the `dmverity-vhd` to compute the root hash
for the new container and update this tool accordingly.
