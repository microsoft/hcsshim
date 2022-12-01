package policy

api_svn := "0.10.0"

mount_device := {"allowed": true}
mount_overlay := {"allowed": true}
create_container := {"allowed": true, "allow_stdio_access": true}
unmount_device := {"allowed": true}
unmount_overlay := {"allowed": true}
exec_in_container := {"allowed": true}
exec_external := {"allowed": true, "allow_stdio_access": true}
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
