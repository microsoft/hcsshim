package policy

api_svn := "0.7.0"

import future.keywords.every
import future.keywords.in

fragments := [
    {"issuer": "did:web:contoso.github.io", "feed": "contoso.azurecr.io/fragment", "minimum_svn": "1.0.0", "includes": ["containers"]},
]
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
    },
    {
        "id": "user_0",
        "command": ["bash", "/copy_resolv_conf.sh"],
        "env_rules": [
            {
              "pattern": "IDENTITY_API_VERSION=.+",
              "strategy": "re2"
            },
            {
              "pattern": "IDENTITY_HEADER=.+",
              "strategy": "re2"
            },
            {
              "pattern": "SOURCE_RESOLV_CONF_LOCATION=/etc/resolv.conf",
              "strategy": "string"
            },
            {
              "pattern": "DESTINATION_RESOLV_CONF_LOCATION=/mount/resolvconf/resolv.conf",
              "strategy": "string"
            },
            {
              "pattern": "IDENTITY_SERVER_THUMBPRINT=.+",
              "strategy": "re2"
            },
            {
              "pattern": "HOSTNAME=.+",
              "strategy": "re2"
            },
            {
              "pattern": "TERM=xterm",
              "strategy": "string"
            },
            {
              "pattern": "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
              "strategy": "string"
            }
       ],
        "layers": [
            "285cb680a55d09f548d4baa804a663764788619824565685b32b8097cbed3d26",
            "a6a6918c07c85e29e48d4a87c1194781251d5185f682c26f20d6ee4e955a239f",
            "296e5baa5b9ded863ca0170e05cd9ecf4136f86c830a9da906184ab147415c7b",
            "97adfda6943f3af972b9bf4fa684f533f10c023d913d195048fef03f9c3c60fd",
            "606fd6baf5eb1a71fd286aea29672a06bfe55f0007ded92ee73142a37590ed19"
        ],

        "mounts": [
            {
              "destination": "/mount/resolvconf",
              "options": ["rbind", "rshared", "rw"],
              "source": "sandbox:///tmp/atlas/resolvconf/.+",
              "type": "bind"
            }
        ],

        "allow_elevated": true,
        "working_dir": "/"
    },
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