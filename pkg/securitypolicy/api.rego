package api

svn := "0.2.0"

enforcement_points := {
    "mount_device": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    "mount_overlay": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    "create_container": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    "unmount_device": {"introducedVersion": "0.2.0", "allowedByDefault": true},
    "exec_in_container": {"introducedVersion": "0.2.0", "allowedByDefault": true},
}
