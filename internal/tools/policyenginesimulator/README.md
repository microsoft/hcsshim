# Policy Engine Simulator

This tool provides a means to test out security policies. Usage:

```
  -commands string
        path to commands JSON file
  -data string
        path to initial data state JSON file (optional)
  -log string
        path to output log file
  -logLevel string
        None|Info|Results|Metadata (default "Info")
  -policy string
        path to policy Rego file
```

## Getting started

From the tool directory run:

   go run . -policy [samples/simple_framework/policy.rego](samples/simple_framework/policy.rego) -commands [samples/simple_framework/commands.json](samples/simple_framework/commands.json)

This will load the authored policy and then simulate the enforcement behavior
for the provided commands.

This policy uses the framework, however, the simulator also handles completely
custom policies:

   go run . -policy [samples/simple_custom/policy.rego](samples/simple_framework/policy.rego) -commands [samples/simple_custom/commands.json](samples/simple_custom/commands.json)

## Commands

Consists of a sequential list of commands that will be issued for enforcement to
the policy. Some sample commands can be seen here:

- [Framework Example Commands](samples/simple_custom/commands.json)
- [Custom Example Commands](samples/simple_framework/commands.json)

These commands take the form of JSON objects, *e.g.*:

``` json
[
    {
        "name": "load_fragment",
        "input": {
            "issuer": "did:web:contoso.github.io",
            "feed": "contoso.azurecr.io/custom",
            "namespace": "custom",
            "local_path": "custom.rego"
        }
    },
    {
        "name": "mount_device",
        "input": {
            "target": "/mnt/layer0",
            "deviceHash": "16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"
        }
    },
    {
        "name": "mount_overlay",
        "input": {
            "target": "/mnt/overlay0",
            "containerID": "container0",
            "layerPaths": [
                "/mnt/layer0"
            ]
        }
    },
    {
        "name": "create_container",
        "input": {
            "containerID": "container0",
            "argList": [
                "/pause"
            ],
            "envList": [
                "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                "TERM=xterm"
            ],
            "mounts": [],
            "workingDir": "/",
            "sandboxDir": "/sandbox",
            "hugePagesDir": "/hugepages"
        }
    }
]
```

Each command has a name (corresponding to the API enforcement point) and then
an input which will be passed directly to the policy. The API being tested
is defined in [`api.rego`](../../../pkg/securitypolicy/api.rego).

## Data

If the authored policy requires certain values in the Rego data structure to
function, or if the author wants to test enforcement behavior given a known
starting state, they can provide an optional initial data file as an argument
to the tool. This should take the form of a single JSON object, *e.g.*:

``` json
{
    "defaultMounts": [/*some mount constraint objects*/],
    "privilegedMounts": [/*some mount constraint objects*/],
    "sandboxPrefix": "sandbox://",
    "hugePagesPrefix": "hugepages://"
}
```

## Log

To aid in debugging, the user can specify a log file. This will enable logging
at the `Info` level, which consists of the output of any Rego `print()` calls
from the policy.

## Log Level

There are several log levels that the user can use to gain greater insight into
policy enforcement:

|    Name    |                       Description                         |
| ---------- | --------------------------------------------------------- |
| `None`     | Used when no log file is provided via the `-log` argument |
| `Info`     | Outputs the results of Rego `print()` statements.         |
| `Results`  | Outputs the results returned by each policy query         |
| `Metadata` | Outputs the entire metadata state after each query        |

## Policy

This can be any Rego with a package name of `policy`, though policies which do
not define the required enforcement point rules will result in enforcement
failures. We include some straightforward samples below, but see
[simple_custom](simple_custom) and [simple_framework](simple_framework)
for more detail.

### Framework-based policy

``` rego
package policy

api_svn := "0.7.0"

import future.keywords.every
import future.keywords.in

containers := [
    {
        "command": ["/pause"],
        "env_rules": [
            {
                "pattern": "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                "strategy": "string",
                "required": false
            },
            {
                "pattern": "TERM=xterm",
                "strategy": "string",
                "required": false
            }
        ],
        "layers": ["16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"],
        "mounts": [],
        "exec_processes": [],
        "signals": [],
        "allow_elevated": false,
        "working_dir": "/"
    }
]

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
load_fragment := data.framework.load_fragment
reason := {"errors": data.framework.errors}
```

### Custom Policy

``` rego
package policy

api_svn := "0.7.0"

overlays := {
    "pause": {
        "deviceHashes": ["16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"],
        "mounts": []
    }
}

custom_containers := [
    {
        "id": "pause",
        "command": ["/pause"],
        "overlayID": "pause",
        "depends": []
    }
]

mount_device := data.custom.mount_device
mount_overlay := data.custom.mount_overlay
create_container := data.custom.create_container
unmount_device := {"allowed": true}
unmount_overlay := {"allowed": true}
exec_in_container := {"allowed": true}
exec_external := {"allowed": true}
shutdown_container := {"allowed": true}
signal_container_process := {"allowed": true}
plan9_mount := {"allowed": true}
plan9_unmount := {"allowed": true}

default load_fragment := {"allowed": false}
load_fragment := {"allowed": true, "add_module": true} {
    input.issuer == "did:web:contoso.github.io"
    input.feed == "contoso.azurecr.io/custom"
}
```