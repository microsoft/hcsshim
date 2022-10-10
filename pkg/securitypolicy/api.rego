package api

svn := "0.9.0"

enforcement_points := {
    "mount_device": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    "mount_overlay": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    "create_container": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    "unmount_device": {"introducedVersion": "0.2.0", "allowedByDefault": true},
    "unmount_overlay": {"introducedVersion": "0.6.0", "allowedByDefault": true},
    "exec_in_container": {"introducedVersion": "0.2.0", "allowedByDefault": true},
    "exec_external": {"introducedVersion": "0.3.0", "allowedByDefault": true},
    "shutdown_container": {"introducedVersion": "0.4.0", "allowedByDefault": true},
    "signal_container_process": {"introducedVersion": "0.5.0", "allowedByDefault": true},
    "plan9_mount": {"introducedVersion": "0.6.0", "allowedByDefault": true},
    "plan9_unmount": {"introducedVersion": "0.6.0", "allowedByDefault": true},
    "get_properties": {"introducedVersion": "0.7.0", "allowedByDefault": true},
    "dump_stacks": {"introducedVersion": "0.7.0", "allowedByDefault": true},
    "runtime_logging": {"introducedVersion": "0.8.0", "allowedByDefault": true},
    "load_fragment": {"introducedVersion": "0.9.0", "allowedByDefault": false},
    "scratch_mount": {"introducedVersion": "0.10.0", "allowedByDefault": true},
    "scratch_unmount": {"introducedVersion": "0.10.0", "allowedByDefault": true},
}
