package api

svn := "0.1.0"

enforcement_points := {
	"mount_device": {"introducedVersion": "0.1.0", "allowedByDefault": false},
	"mount_overlay": {"introducedVersion": "0.1.0", "allowedByDefault": false},
	"create_container": {"introducedVersion": "0.1.0", "allowedByDefault": false},
    # the following rules are used for testing the default behavior logic. DO NOT REMOVE.
    "__fixture_for_future_test__": {"introducedVersion": "100.0.0", "allowedByDefault": true},
    "__fixture_for_allowed_test_true__": {"introducedVersion": "0.0.2", "allowedByDefault": true},
    "__fixture_for_allowed_test_false__": {"introducedVersion": "0.0.2", "allowedByDefault": false},
}

default enforcement_point_info := {"available": false, "allowed": false, "unknown": true, "invalid": false}

enforcement_point_info := {"available": available, "allowed": allowed, "unknown": false, "invalid": false} {
	enforcement_point := enforcement_points[input.name]
	semver.compare(svn, enforcement_point.introducedVersion) >= 0
	available := semver.compare(data.policy.api_svn, enforcement_point.introducedVersion) >= 0
	allowed := enforcement_point.allowedByDefault
}

enforcement_point_info := {"available": false, "allowed": false, "unknown": false, "invalid": true} {
	enforcement_point := enforcement_points[input.name]
	semver.compare(svn, enforcement_point.introducedVersion) < 0
}
