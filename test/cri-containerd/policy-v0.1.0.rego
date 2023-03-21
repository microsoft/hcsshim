package policy

api_svn := "0.10.0"
framework_svn := "0.1.0"

containers := [
    {
        "command": ["/pause"],
        "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false}],
        "layers": ["16b514057a06ad665f92c02863aca074fd5976c755d26bff16365299169e8415"],
        "mounts": [],
        "exec_processes": [],
        "signals": [],
        "allow_elevated": false,
        "working_dir": "/",
        "allow_stdio_access": false,
    },
    {
        "command": ["ash","-c","echo 'Hello'"],
        "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true},{"pattern": `TERM=xterm`, "strategy": "string", "required": false}],
        "layers": ["0196c8ecca6c885a250b186ed8449cf42d737b8551b8d0839381461d95461802"],
        "mounts": [],
        "exec_processes": [],
        "signals": [],
        "allow_elevated": true,
        "working_dir": "/",
        "allow_stdio_access": false,
    },
]
external_processes := [
    {"command": ["ls","-l","/dev/mapper"], "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true}], "working_dir": "/", "allow_stdio_access": true},
    {"command": ["bash"], "env_rules": [{"pattern": `PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, "strategy": "string", "required": true}], "working_dir": "/", "allow_stdio_access": true},
]
allow_properties_access := true
allow_dump_stacks := true
allow_runtime_logging := true
allow_environment_variable_dropping := false
allow_unencrypted_scratch := true


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
    "matches": data.framework.matches,
}
