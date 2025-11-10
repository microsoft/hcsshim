package api

version := "@@API_VERSION@@"

enforcement_points := {
    "mount_device": {"introducedVersion": "0.1.0", "default_results": {"allowed": false}, "use_framework": false},
    "rw_mount_device": {"introducedVersion": "0.11.0", "default_results": {}, "use_framework": true},
    "mount_overlay": {"introducedVersion": "0.1.0", "default_results": {"allowed": false}, "use_framework": false},
    "mount_cims": {"introducedVersion": "0.11.0", "default_results": {"allowed": false}, "use_framework": false},
    "create_container": {"introducedVersion": "0.1.0", "default_results": {"allowed": false, "env_list": null, "allow_stdio_access": false}, "use_framework": false},
    "unmount_device": {"introducedVersion": "0.2.0", "default_results": {"allowed": true}, "use_framework": false},
    "rw_unmount_device": {"introducedVersion": "0.11.0", "default_results": {}, "use_framework": true},
    "unmount_overlay": {"introducedVersion": "0.6.0", "default_results": {"allowed": true}, "use_framework": false},
    "exec_in_container": {"introducedVersion": "0.2.0", "default_results": {"allowed": true, "env_list": null}, "use_framework": false},
    "exec_external": {"introducedVersion": "0.3.0", "default_results": {"allowed": true, "env_list": null, "allow_stdio_access": false}, "use_framework": false},
    "shutdown_container": {"introducedVersion": "0.4.0", "default_results": {"allowed": true}, "use_framework": false},
    "signal_container_process": {"introducedVersion": "0.5.0", "default_results": {"allowed": true}, "use_framework": false},
    "plan9_mount": {"introducedVersion": "0.6.0", "default_results": {"allowed": true}, "use_framework": false},
    "plan9_unmount": {"introducedVersion": "0.6.0", "default_results": {"allowed": true}, "use_framework": false},
    "get_properties": {"introducedVersion": "0.7.0", "default_results": {"allowed": true}, "use_framework": false},
    "dump_stacks": {"introducedVersion": "0.7.0", "default_results": {"allowed": true}, "use_framework": false},
    "runtime_logging": {"introducedVersion": "0.8.0", "default_results": {"allowed": true}, "use_framework": false},
    "load_fragment": {"introducedVersion": "0.9.0", "default_results": {"allowed": false, "add_module": false}, "use_framework": false},
    "scratch_mount": {"introducedVersion": "0.10.0", "default_results": {"allowed": true}, "use_framework": false},
    "scratch_unmount": {"introducedVersion": "0.10.0", "default_results": {"allowed": true}, "use_framework": false},
}
