package policy

api_version := "@@API_VERSION@@"
framework_version := "@@FRAMEWORK_VERSION@@"

fragments := [
  {
    "feed": "@@FRAGMENT_FEED@@",
    "includes": [
      "containers",
      "fragments"
    ],
    "issuer": "@@FRAGMENT_ISSUER@@",
    "minimum_svn": "0"
  }
]

platform_rules := [
  {
    "env_rules": [
      {
        "name": "(?i)(FABRIC)_.+",
        "name_strategy": "re2",
        "value": ".+",
        "value_strategy": "re2"
      }
    ],
    "mounts": [
      {
        "destination": "/var/run/secrets/kubernetes.io/serviceaccount",
        "options": [
          "rbind",
          "rshared",
          "ro"
        ],
        "source": "sandbox:///tmp/atlas/emptydir/.+",
        "type": "bind"
      }
    ]
  }
]

containers := [
  {
    "allow_elevated": false,
    "allow_stdio_access": true,
    "capabilities": {
      "ambient": [],
      "bounding": [],
      "effective": [],
      "inheritable": [],
      "permitted": []
    },
    "command": [ "bash" ],
    "env_rules": [],
    "exec_processes": [],
    "layers": [
      "0000000000000000000000000000000000000000000000000000000000000000",
    ],
    "mounts": [],
    "no_new_privileges": false,
    "seccomp_profile_sha256": "",
    "signals": [],
    "user": {
      "group_idnames": [
        {
          "pattern": "",
          "strategy": "any"
        }
      ],
      "umask": "0022",
      "user_idname": {
        "pattern": "",
        "strategy": "any"
      }
    },
    "working_dir": "/"
  }
]

allow_properties_access := true
allow_dump_stacks := false
allow_runtime_logging := false
allow_environment_variable_dropping := true
allow_unencrypted_scratch := false
allow_capability_dropping := true

mount_device := data.framework.mount_device
rw_mount_device := data.framework.rw_mount_device
unmount_device := data.framework.unmount_device
rw_unmount_device := data.framework.rw_unmount_device
mount_overlay := data.framework.mount_overlay
unmount_overlay := data.framework.unmount_overlay
mount_cims := data.framework.mount_cims
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
reason := data.framework.reason
