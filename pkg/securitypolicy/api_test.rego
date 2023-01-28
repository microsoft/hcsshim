package api

svn := "0.0.2"

enforcement_points := {
    "__fixture_for_future_test__": {"introducedVersion": "100.0.0", "default_results": {"allowed": true}},
    "__fixture_for_allowed_test_true__": {"introducedVersion": "0.0.2", "default_results": {"allowed": true}},
    "__fixture_for_allowed_test_false__": {"introducedVersion": "0.0.2", "default_results": {"allowed": false}},
    "__fixture_for_allowed_extra__": {"introducedVersion": "0.0.1", "default_results": {"allowed": false, "__test__": "test"}}
}
