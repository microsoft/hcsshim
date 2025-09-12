package policy

api_version := "@@API_VERSION@@"

mount_device := {"allowed": true}
mount_overlay := {"allowed": true}
create_container := {"allowed": true, "env_list": null, "allow_stdio_access": true}
mount_cims := {"allowed": true}
unmount_device := {"allowed": true}
unmount_overlay := {"allowed": true}
exec_in_container := {"allowed": true, "env_list": null}
exec_external := {"allowed": true, "env_list": null, "allow_stdio_access": true}
shutdown_container := {"allowed": true}
signal_container_process := {"allowed": true}
plan9_mount := {"allowed": true}
plan9_unmount := {"allowed": true}
get_properties := {"allowed": true}
dump_stacks := {"allowed": true}
runtime_logging := {"allowed": true}
load_fragment := {"allowed": true}
scratch_mount := {"allowed": true}
scratch_unmount := {"allowed": true}
