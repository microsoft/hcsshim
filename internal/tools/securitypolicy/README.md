# securitypolicy

Takes a configuration to a TOML file and outputs a Base64 encoded string of the
generated security policy.

`securitypolicy` exists as a tool to make it easier to generate security policies
for developers working functionality related to security policy in this repository.
It is not intended to be used by "end users" but could be used as a basis for
such a tool.

A Base64 encoded version of policy is sent as an annotation to GCS for processing.
The `securitypolicy` tool outputs a Base64 encoded Rego policy.

Running the tool can take a long time as each layer for each container must
be downloaded, turned into an ext4, and finally a dm-verity root hash calculated.

For C-WCOW policies, use `-os windows`. Like C-LCOW, the input is image-based:
give an `image_name` and the tool computes the verified Block CIM layer digests
and the merged CIM digest for you (via `ImportBlockCIMLayerWithOpts` +
`MergeBlockCIMLayersWithOpts`). This computation uses the Windows CIM APIs, so
`-os windows` must run on Windows (and typically elevated). Windows policies
support Rego output only.

A single config file can hold both `[[container]]` (C-LCOW) and
`[[windows_container]]` (C-WCOW) sections. The `-os` flag selects which section
is used; the other OS's container entries are ignored.

## C-WCOW example

```toml
[[windows_container]]
image_name = "mcr.microsoft.com/windows/nanoserver:ltsc2022"
command = ["cmd.exe", "/c", "echo hello"]
working_dir = "C:\\"
user = "ContainerUser"
allow_stdio_access = true

[[windows_container.env_rule]]
strategy = "string"
rule = "PATH=C:\\Windows\\system32;C:\\Windows"
required = true
```

Generate a raw and Base64-encoded Rego policy (on Windows):

  securitypolicytool -c windows.toml -os windows -t rego -r

`-t fragment` also works with `-os windows` to emit a Windows policy fragment.

## Example TOML configuration file

```toml
allow_capability_dropping = true

[[container]]
image_name = "rust:1.52.1"
command = ["rustc", "--help"]
working_dir = "/home/user"
allow_elevated = true

[container.capabilities]
bounding = ["CAP_SYS_ADMIN"]
effective = ["CAP_SYS_ADMIN"]
inheritable = ["CAP_SYS_ADMIN"]
permitted = ["CAP_SYS_ADMIN"]
ambient = ["CAP_SYS_ADMIN"]

[[container.env_rule]]
strategy = "re2"
rule = "PREFIX_.+=.+"

[[container.mount]]
host_path = "sandbox:///host/path/one"
container_path = "/container/path/one"
readonly = false

[[container.mount]]
host_path = "sandbox:///host/path/two"
container_path = "/container/path/two"
readonly = true

[[container.exec_process]]
command = ["top"]
working_dir = "/home/user"

[[container.exec_process.env_rule]]
strategy = "string"
rule = "FOO=bar"

[[external_process]]
command = ["bash"]
working_dir = "/"

[[fragment]]
issuer = "did:web:contoso.com"
feed = "contoso.azurecr.io/infra"
minimum_svn = "1"
include = ["containers"]
```

## Converted to Rego Policy

The result of the command:

    securitypolicytool -c sample.toml -t rego -r

Is the following Rego policy:

``` rego
package policy

api_version := "0.10.0"
framework_version := "0.3.0"

fragments := [
    {"issuer": "did:web:contoso.com", "feed": "contoso.azurecr.io/infra", "minimum_svn": 1, "includes": ["containers"]},
]
containers := [
    {
        "command": ["rustc","--help"],
        "env_rules": [{"pattern": `PATH=/usr/local/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `RUSTUP_HOME=/usr/local/rustup`, "strategy": "string", "required": true},{"pattern": `CARGO_HOME=/usr/local/cargo`, "strategy": "string", "required": true},{"pattern": `RUST_VERSION=1.52.1`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false},{"pattern": `PREFIX_.+=.+`, "strategy": "re2", "required": false}],
        "layers": ["fe84c9d5bfddd07a2624d00333cf13c1a9c941f3a261f13ead44fc6a93bc0e7a","4dedae42847c704da891a28c25d32201a1ae440bce2aecccfa8e6f03b97a6a6c","41d64cdeb347bf236b4c13b7403b633ff11f1cf94dbc7cf881a44d6da88c5156","eb36921e1f82af46dfe248ef8f1b3afb6a5230a64181d960d10237a08cd73c79","e769d7487cc314d3ee748a4440805317c19262c7acd2fdbdb0d47d2e4613a15c","1b80f120dbd88e4355d6241b519c3e25290215c469516b49dece9cf07175a766"],
        "mounts": [{"destination": "/container/path/one", "options": ["rbind","rshared","rw"], "source": "sandbox:///host/path/one", "type": "bind"},{"destination": "/container/path/two", "options": ["rbind","rshared","ro"], "source": "sandbox:///host/path/two", "type": "bind"}],
        "exec_processes": [{"command": ["top"], "signals": []}],
        "signals": [],
        "user": {
            "user_idname": {"pattern": ``, "strategy": "any"},
            "group_idnames": [{"pattern": ``, "strategy": "any"}],
            "umask": "0022"
        },
        "capabilities": {
            "bounding": ["CAP_SYS_ADMIN"],
            "effective": ["CAP_SYS_ADMIN"],
            "inheritable": ["CAP_SYS_ADMIN"],
            "permitted": ["CAP_SYS_ADMIN"],
            "ambient": ["CAP_SYS_ADMIN"],
        },
        "seccomp_profile_sha256": "",
        "allow_elevated": true,
        "working_dir": "/home/user",
        "allow_stdio_access": false,
        "no_new_privileges": true,
    },
    {
        "command": ["/pause"],
        "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false}],
        "layers": ["16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"],
        "mounts": [],
        "exec_processes": [],
        "signals": [],
        "user": {
            "user_idname": {"pattern": ``, "strategy": "any"},
            "group_idnames": [{"pattern": ``, "strategy": "any"}],
            "umask": "0022"
        },
        "capabilities": null,
        "seccomp_profile_sha256": "",
        "allow_elevated": false,
        "working_dir": "/",
        "allow_stdio_access": false,
        "no_new_privileges": true,
    },
]
external_processes := [
    {"command": ["bash"], "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true}], "working_dir": "/", "allow_stdio_access": false},
]
allow_properties_access := false
allow_dump_stacks := false
allow_runtime_logging := false
allow_environment_variable_dropping := false
allow_unencrypted_scratch := false
allow_capability_dropping := true


mount_device := data.framework.mount_device
unmount_device := data.framework.unmount_device
mount_overlay := data.framework.mount_overlay
unmount_overlay := data.framework.unmount_overlay
create_container := data.framework.create_container
exec_in_container := data.framework.exec_in_container
exec_external := data.framework.exec_external
shutdown_container := data.framework.shutdown_container
signal_container_process := data.framework.signal_container_process
plan9_mount := data.framework.plan9_mount
plan9_unmount := data.framework.plan9_unmount
get_properties := data.framework.get_properties
dump_stacks := data.framework.dump_stacks
runtime_logging := data.framework.runtime_logging
load_fragment := data.framework.load_fragment
scratch_mount := data.framework.scratch_mount
scratch_unmount := data.framework.scratch_unmount
reason := {
    "errors": data.framework.errors,
    "error_objects": data.framework.error_objects,
}
```

## Converted to Rego Fragment

The result of the command

    securitypolicytool -c sample.toml -t fragment -n sample -v 1 -r

is the following Rego fragment:

``` rego
package sample

svn := 1
framework_version := "0.3.0"

fragments := [
    {"issuer": "did:web:contoso.com", "feed": "contoso.azurecr.io/infra", "minimum_svn": 1, "includes": ["containers"]},
]
containers := [
    {
        "command": ["rustc","--help"],
        "env_rules": [{"pattern": `PATH=/usr/local/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `RUSTUP_HOME=/usr/local/rustup`, "strategy": "string", "required": true},{"pattern": `CARGO_HOME=/usr/local/cargo`, "strategy": "string", "required": true},{"pattern": `RUST_VERSION=1.52.1`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false},{"pattern": `PREFIX_.+=.+`, "strategy": "re2", "required": false}],
        "layers": ["fe84c9d5bfddd07a2624d00333cf13c1a9c941f3a261f13ead44fc6a93bc0e7a","4dedae42847c704da891a28c25d32201a1ae440bce2aecccfa8e6f03b97a6a6c","41d64cdeb347bf236b4c13b7403b633ff11f1cf94dbc7cf881a44d6da88c5156","eb36921e1f82af46dfe248ef8f1b3afb6a5230a64181d960d10237a08cd73c79","e769d7487cc314d3ee748a4440805317c19262c7acd2fdbdb0d47d2e4613a15c","1b80f120dbd88e4355d6241b519c3e25290215c469516b49dece9cf07175a766"],
        "mounts": [{"destination": "/container/path/one", "options": ["rbind","rshared","rw"], "source": "sandbox:///host/path/one", "type": "bind"},{"destination": "/container/path/two", "options": ["rbind","rshared","ro"], "source": "sandbox:///host/path/two", "type": "bind"}],
        "exec_processes": [{"command": ["top"], "signals": []}],
        "signals": [],
        "user": {
            "user_idname": {"pattern": ``, "strategy": "any"},
            "group_idnames": [{"pattern": ``, "strategy": "any"}],
            "umask": "0022"
        },
        "capabilities": {
            "bounding": ["CAP_SYS_ADMIN"],
            "effective": ["CAP_SYS_ADMIN"],
            "inheritable": ["CAP_SYS_ADMIN"],
            "permitted": ["CAP_SYS_ADMIN"],
            "ambient": ["CAP_SYS_ADMIN"],
        },
        "seccomp_profile_sha256": "",
        "allow_elevated": true,
        "working_dir": "/home/user",
        "allow_stdio_access": false,
        "no_new_privileges": true,
    },
    {
        "command": ["/pause"],
        "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false}],
        "layers": ["16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"],
        "mounts": [],
        "exec_processes": [],
        "signals": [],
        "user": {
            "user_idname": {"pattern": ``, "strategy": "any"},
            "group_idnames": [{"pattern": ``, "strategy": "any"}],
            "umask": "0022"
        },
        "capabilities": null,
        "seccomp_profile_sha256": "",
        "allow_elevated": false,
        "working_dir": "/",
        "allow_stdio_access": false,
        "no_new_privileges": true,
    },
]
external_processes := [
    {"command": ["bash"], "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true}], "working_dir": "/", "allow_stdio_access": false},
]
```

## CLI Options

### `-c`

TOML configuration file to process (required)

### `-r`

output raw marshaled policy in addition to the base64

### `-t`

one of:
- `rego`: outputs a Rego policy
- `fragment`: outputs a Rego fragment. The `-n` and `-v` are required for this option.

### `-n`

Required for `-t fragment`. Specifies the fragment Rego namespace.

### `-v`

Required for `-t fragment`. Specified the fragment SVN as a monotonically increasing integer.
*e.g.*, 1.
Typically only incremented for security relevent changes.

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
the hardcoded root hash by running the [dmverity-vhd](https://github.com/microsoft/integrity-vhd/tree/main/cmd/dmverity-vhd)
to compute the root hash for the new container and update this tool accordingly.
