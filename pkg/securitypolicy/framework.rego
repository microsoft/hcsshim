package framework

import future.keywords.every
import future.keywords.in

svn := "@@FRAMEWORK_SVN@@"

device_mounted(target) {
    data.metadata.devices[target]
}

default deviceHash_ok := false

# test if a device hash exists as a layer in a policy container
deviceHash_ok {
    layer := data.policy.containers[_].layers[_]
    input.deviceHash == layer
}

# test if a device hash exists as a layer in a fragment container
deviceHash_ok {
    feed := data.metadata.issuers[_].feeds[_]
    some fragment in feed
    layer := fragment.containers[_].layers[_]
    input.deviceHash == layer
}

default mount_device := {"allowed": false}

mount_device := {"metadata": [addDevice], "allowed": true} {
    not device_mounted(input.target)
    deviceHash_ok
    addDevice := {
        "name": "devices",
        "action": "add",
        "key": input.target,
        "value": input.deviceHash,
    }
}

default unmount_device := {"allowed": false}

unmount_device := {"metadata": [removeDevice], "allowed": true} {
    device_mounted(input.unmountTarget)
    removeDevice := {
        "name": "devices",
        "action": "remove",
        "key": input.unmountTarget,
    }
}

layerPaths_ok(layers) {
    length := count(layers)
    count(input.layerPaths) == length
    every i, path in input.layerPaths {
        layers[(length - i) - 1] == data.metadata.devices[path]
    }
}

default overlay_exists := false

overlay_exists {
    data.metadata.matches[input.containerID]
}

overlay_mounted(target) {
    data.metadata.overlayTargets[target]
}

default candidate_containers := []

candidate_containers := containers {
    semver.compare(data.policy.framework_svn, svn) == 0

    policy_containers := [c | c := data.policy.containers[_]]
    fragment_containers := [c |
        feed := data.metadata.issuers[_].feeds[_]
        fragment := feed[_]
        c := fragment.containers[_]
    ]

    containers := array.concat(policy_containers, fragment_containers)
}

candidate_containers := containers {
    semver.compare(data.policy.framework_svn, svn) < 0

    policy_containers := apply_defaults("container", data.policy.containers, data.policy.framework_svn)
    fragment_containers := [c |
        feed := data.metadata.issuers[_].feeds[_]
        fragment := feed[_]
        c := fragment.containers[_]
    ]

    containers := array.concat(policy_containers, fragment_containers)
}

default mount_overlay := {"allowed": false}

mount_overlay := {"metadata": [addMatches, addOverlayTarget], "allowed": true} {
    not overlay_exists

    containers := [container |
        container := candidate_containers[_]
        layerPaths_ok(container.layers)
    ]

    count(containers) > 0
    addMatches := {
        "name": "matches",
        "action": "add",
        "key": input.containerID,
        "value": containers,
    }

    addOverlayTarget := {
        "name": "overlayTargets",
        "action": "add",
        "key": input.target,
        "value": true,
    }
}

default unmount_overlay := {"allowed": false}

unmount_overlay := {"metadata": [removeOverlayTarget], "allowed": true} {
    overlay_mounted(input.unmountTarget)
    removeOverlayTarget := {
        "name": "overlayTargets",
        "action": "remove",
        "key": input.unmountTarget,
    }
}

command_ok(command) {
    count(input.argList) == count(command)
    every i, arg in input.argList {
        command[i] == arg
    }
}

env_ok(pattern, "string", value) {
    pattern == value
}

env_ok(pattern, "re2", value) {
    regex.match(pattern, value)
}

rule_ok(rule, env) {
    not rule.required
}

rule_ok(rule, env) {
    rule.required
    env_ok(rule.pattern, rule.strategy, env)
}

envList_ok(env_rules, envList) {
    every rule in env_rules {
        some env in envList
        rule_ok(rule, env)
    }

    every env in envList {
        some rule in env_rules
        env_ok(rule.pattern, rule.strategy, env)
    }
}

valid_envs_subset(env_rules) := envs {
    envs := {env |
        some env in input.envList
        some rule in env_rules
        env_ok(rule.pattern, rule.strategy, env)
    }
}

valid_envs_for_all(items) := envs {
    data.policy.allow_environment_variable_dropping

    # for each item, find a subset of the environment rules
    # that are valid
    valid := [envs |
        some item in items
        envs := valid_envs_subset(item.env_rules)
    ]

    # we want to select the most specific matches, which in this
    # case consists of those matches which require dropping the
    # fewest environment variables (i.e. the longest lists)
    counts := [num_envs |
        envs := valid[_]
        num_envs := count(envs)
    ]
    max_count := max(counts)

    largest_env_sets := {envs |
        some i
        counts[i] == max_count
        envs := valid[i]
    }

    # if there is more than one set with the same size, we
    # can only proceed if they are all the same, so we verify
    # that the intersection is equal to the union. For a single
    # set this is trivially true.
    envs_i := intersection(largest_env_sets)
    envs_u := union(largest_env_sets)
    envs_i == envs_u
    envs := envs_i
}

valid_envs_for_all(items) := envs {
    not data.policy.allow_environment_variable_dropping

    # no dropping allowed, so we just return the input
    envs := input.envList
}

workingDirectory_ok(working_dir) {
    input.workingDir == working_dir
}

privileged_ok(elevation_allowed) {
    not input.privileged
}

privileged_ok(elevation_allowed) {
    input.privileged
    input.privileged == elevation_allowed
}

default container_started := false

container_started {
    data.metadata.started[input.containerID]
}

default create_container := {"allowed": false}

create_container := {"metadata": [updateMatches, addStarted],
                     "env_list": env_list,
                     "allow_stdio_access": allow_stdio_access,
                     "allowed": true} {
    not container_started

    # narrow the matches based upon command, working directory, and
    # mount list
    possible_containers := [container |
        container := data.metadata.matches[input.containerID][_]
        privileged_ok(container.allow_elevated)
        workingDirectory_ok(container.working_dir)
        command_ok(container.command)
        mountList_ok(container.mounts, container.allow_elevated)
    ]

    count(possible_containers) > 0

    # check to see if the environment variables match, dropping
    # them if allowed (and necessary)
    env_list := valid_envs_for_all(possible_containers)
    containers := [container |
        container := possible_containers[_]
        envList_ok(container.env_rules, env_list)
    ]

    count(containers) > 0

    # we can't do narrowing based on allowing stdio access so at this point
    # every container from the policy that might match this create request
    # must have the same allow stdio value otherwise, we are in an undecidable
    # state
    allow_stdio_access := containers[0].allow_stdio_access
    every c in containers {
        c.allow_stdio_access == allow_stdio_access
    }

    updateMatches := {
        "name": "matches",
        "action": "update",
        "key": input.containerID,
        "value": containers,
    }

    addStarted := {
        "name": "started",
        "action": "add",
        "key": input.containerID,
        "value": true,
    }
}

mountSource_ok(constraint, source) {
    startswith(constraint, data.sandboxPrefix)
    newConstraint := replace(constraint, data.sandboxPrefix, input.sandboxDir)
    regex.match(newConstraint, source)
}

mountSource_ok(constraint, source) {
    startswith(constraint, data.hugePagesPrefix)
    newConstraint := replace(constraint, data.hugePagesPrefix, input.hugePagesDir)
    regex.match(newConstraint, source)
}

mountSource_ok(constraint, source) {
    startswith(constraint, data.plan9Prefix)
    some target, containerID in data.metadata.p9mounts
    source == target
    input.containerID == containerID
}

mountSource_ok(constraint, source) {
    constraint == source
}

mountConstraint_ok(constraint, mount) {
    mount.type == constraint.type
    mountSource_ok(constraint.source, mount.source)
    mount.destination != ""
    mount.destination == constraint.destination

    # the following check is not required (as the following tests will prove this
    # condition as well), however it will check whether those more expensive
    # tests need to be performed.
    count(mount.options) == count(constraint.options)
    every option in mount.options {
        some constraintOption in constraint.options
        option == constraintOption
    }

    every option in constraint.options {
        some mountOption in mount.options
        option == mountOption
    }
}

mount_ok(mounts, allow_elevated, mount) {
    some constraint in mounts
    mountConstraint_ok(constraint, mount)
}

mount_ok(mounts, allow_elevated, mount) {
    some constraint in data.defaultMounts
    mountConstraint_ok(constraint, mount)
}

mount_ok(mounts, allow_elevated, mount) {
    allow_elevated
    some constraint in data.privilegedMounts
    mountConstraint_ok(constraint, mount)
}

mountList_ok(mounts, allow_elevated) {
    every mount in input.mounts {
        mount_ok(mounts, allow_elevated, mount)
    }
}

default exec_in_container := {"allowed": false}

exec_in_container := {"metadata": [updateMatches],
                      "env_list": env_list,
                      "allowed": true} {
    container_started

    # narrow our matches based upon the process requested
    possible_containers := [container |
        container := data.metadata.matches[input.containerID][_]
        workingDirectory_ok(container.working_dir)
        some process in container.exec_processes
        command_ok(process.command)
    ]

    count(possible_containers) > 0

    # check to see if the environment variables match, dropping
    # them if allowed (and necessary)
    env_list := valid_envs_for_all(possible_containers)
    containers := [container |
        container := possible_containers[_]
        envList_ok(container.env_rules, env_list)
    ]

    count(containers) > 0
    updateMatches := {
        "name": "matches",
        "action": "update",
        "key": input.containerID,
        "value": containers,
    }
}

default shutdown_container := {"allowed": false}

shutdown_container := {"started": remove, "metadata": [remove], "allowed": true} {
    container_started
    remove := {
        "name": "matches",
        "action": "remove",
        "key": input.containerID,
    }
}

default signal_container_process := {"allowed": false}

signal_container_process := {"metadata": [updateMatches], "allowed": true} {
    container_started
    input.isInitProcess
    containers := [container |
        container := data.metadata.matches[input.containerID][_]
        signal_ok(container.signals)
    ]

    count(containers) > 0
    updateMatches := {
        "name": "matches",
        "action": "update",
        "key": input.containerID,
        "value": containers,
    }
}

signal_container_process := {"metadata": [updateMatches], "allowed": true} {
    container_started
    not input.isInitProcess
    containers := [container |
        container := data.metadata.matches[input.containerID][_]
        some process in container.exec_processes
        command_ok(process.command)
        signal_ok(process.signals)
    ]

    count(containers) > 0
    updateMatches := {
        "name": "matches",
        "action": "update",
        "key": input.containerID,
        "value": containers,
    }
}

signal_ok(signals) {
    some signal in signals
    input.signal == signal
}

plan9_mounted(target) {
    data.metadata.p9mounts[target]
}

default plan9_mount := {"allowed": false}

plan9_mount := {"metadata": [addPlan9Target], "allowed": true} {
    not plan9_mounted(input.target)
    some containerID, _ in data.metadata.matches
    pattern := concat("", [input.rootPrefix, "/", containerID, input.mountPathPrefix])
    regex.match(pattern, input.target)
    addPlan9Target := {
        "name": "p9mounts",
        "action": "add",
        "key": input.target,
        "value": containerID,
    }
}

default plan9_unmount := {"allowed": false}

plan9_unmount := {"metadata": [removePlan9Target], "allowed": true} {
    plan9_mounted(input.unmountTarget)
    removePlan9Target := {
        "name": "p9mounts",
        "action": "remove",
        "key": input.unmountTarget,
    }
}


default enforcement_point_info := {"available": false, "default_results": {"allow": false}, "unknown": true, "invalid": false}

enforcement_point_info := {"available": available, "default_results": default_results, "unknown": false, "invalid": false} {
    enforcement_point := data.api.enforcement_points[input.name]
    semver.compare(data.api.svn, enforcement_point.introducedVersion) >= 0
    available := semver.compare(data.policy.api_svn, enforcement_point.introducedVersion) >= 0
    default_results := enforcement_point.default_results
}

enforcement_point_info := {"available": false, "default_results": {"allow": false}, "unknown": false, "invalid": true} {
    enforcement_point := data.api.enforcement_points[input.name]
    semver.compare(data.api.svn, enforcement_point.introducedVersion) < 0
}

default candidate_external_processes := []

candidate_external_processes := external_processes {
    semver.compare(data.policy.framework_svn, svn) == 0

    policy_external_processes := [e | e := data.policy.external_processes[_]]
    fragment_external_processes := [e |
        feed := data.metadata.issuers[_].feeds[_]
        fragment := feed[_]
        e := fragment.external_processes[_]
    ]

    external_processes := array.concat(policy_external_processes, fragment_external_processes)
}

candidate_external_processes := external_processes {
    semver.compare(data.policy.framework_svn, svn) < 0

    policy_external_processes := apply_defaults("external_process", data.policy.external_processes, data.policy.framework_svn)
    fragment_external_processes := [e |
        feed := data.metadata.issuers[_].feeds[_]
        fragment := feed[_]
        e := fragment.external_processes[_]
    ]

    external_processes := array.concat(policy_external_processes, fragment_external_processes)
}

external_process_ok(process) {
    command_ok(process.command)
    envList_ok(process.env_rules, input.envList)
    workingDirectory_ok(process.working_dir)
}

default exec_external := {"allowed": false}

exec_external := {"allowed": true,
                  "allow_stdio_access": allow_stdio_access,
                  "env_list": env_list} {
    print(count(candidate_external_processes))

    possible_processes := [process |
        process := candidate_external_processes[_]
        workingDirectory_ok(process.working_dir)
        command_ok(process.command)
    ]

    print(count(possible_processes))
    # check to see if the environment variables match, dropping
    # them if allowed (and necessary)
    env_list := valid_envs_for_all(possible_processes)
    processes := [process |
        process := possible_processes[_]
        envList_ok(process.env_rules, env_list)
    ]

    count(processes) > 0

    allow_stdio_access := processes[0].allow_stdio_access
    every p in processes {
        p.allow_stdio_access == allow_stdio_access
    }
}

default get_properties := {"allowed": false}

get_properties := {"allowed": true} {
    data.policy.allow_properties_access
}

default dump_stacks := {"allowed": false}

dump_stacks := {"allowed": true} {
    data.policy.allow_dump_stacks
}

default runtime_logging := {"allowed": false}

runtime_logging := {"allowed": true} {
    data.policy.allow_runtime_logging
}

default fragment_containers := []

fragment_containers := data[input.namespace].containers

default fragment_fragments := []

fragment_fragments := data[input.namespace].fragments

default fragment_external_processes := []

fragment_external_processes := data[input.namespace].external_processes

apply_defaults(name, raw_values, framework_svn) := values {
    semver.compare(framework_svn, svn) == 0
    values := raw_values
}

apply_defaults(name, raw_values, framework_svn) := values {
    semver.compare(framework_svn, svn) < 0
    template := load_defaults(name, framework_svn)
    print(template)
    values := [updated |
        raw := raw_values[_]
        flat := object.union(template, raw)
        arrays := {key: values |
            template[key]["__array__"]
            item_template := template[key]["item"]
            values := [object.union(item_template, raw) | raw := raw[key][_]]
        }
        updated := object.union(flat, arrays)
    ]
}

extract_fragment_includes(includes) := fragment {
    framework_svn := data[input.namespace].framework_svn
    objects := {
        "containers": apply_defaults("container", fragment_containers, framework_svn),
        "fragments": apply_defaults("fragment", fragment_fragments, framework_svn),
        "external_processes": apply_defaults("external_process", fragment_external_processes, framework_svn)
    }

    fragment := {
        include: objects[include] | include := includes[_]
    }
}

issuer_exists(iss) {
    data.metadata.issuers[iss]
}

feed_exists(iss, feed) {
    data.metadata.issuers[iss].feeds[feed]
}

update_issuer(includes) := issuer {
    feed_exists(input.issuer, input.feed)
    old_issuer := data.metadata.issuers[input.issuer]
    old_fragments := old_issuer.feeds[input.feed]
    new_issuer := {"feeds": {input.feed: array.concat([extract_fragment_includes(includes)], old_fragments)}}

    issuer := object.union(old_issuer, new_issuer)
}

update_issuer(includes) := issuer {
    not feed_exists(input.issuer, input.feed)
    old_issuer := data.metadata.issuers[input.issuer]
    new_issuer := {"feeds": {input.feed: [extract_fragment_includes(includes)]}}

    issuer := object.union(old_issuer, new_issuer)
}

update_issuer(includes) := issuer {
    not issuer_exists(input.issuer)
    issuer := {"feeds": {input.feed: [extract_fragment_includes(includes)]}}
}

default candidate_fragments := []

candidate_fragments := fragments {
    semver.compare(data.policy.framework_svn, svn) == 0

    policy_fragmemnts := [f | f := data.policy.fragments[_]]
    fragment_fragments := [f |
        feed := data.metadata.issuers[_].feeds[_]
        fragment := feed[_]
        f := fragment.fragments[_]
    ]

    fragments := array.concat(policy_fragmemnts, fragment_fragments)
}

candidate_fragments := fragments {
    semver.compare(data.policy.framework_svn, svn) < 0

    policy_fragments := apply_defaults("fragment", data.policy.fragments, data.policy.framework_svn)
    fragment_fragments := [f |
        feed := data.metadata.issuers[_].feeds[_]
        fragment := feed[_]
        f := fragment.fragments[_]
    ]

    fragments := array.concat(policy_fragments, fragment_fragments)
}

default load_fragment := {"allowed": false}

fragment_ok(fragment) {
    input.issuer == fragment.issuer
    input.feed == fragment.feed
    semver.compare(data[input.namespace].svn, fragment.minimum_svn) >= 0
}

load_fragment := {"metadata": [updateIssuer], "add_module": add_module, "allowed": true} {
    some fragment in candidate_fragments
    fragment_ok(fragment)

    issuer := update_issuer(fragment.includes)
    updateIssuer := {
        "name": "issuers",
        "action": "update",
        "key": input.issuer,
        "value": issuer,
    }

    add_module := "namespace" in fragment.includes
}

default scratch_mount := {"allowed": false}

scratch_mounted(target) {
    data.metadata.scratch_mounts[target]
}

scratch_mount := {"metadata": [add_scratch_mount], "allowed": true} {
    not scratch_mounted(input.target)
    data.policy.allow_unencrypted_scratch
    add_scratch_mount := {
        "name": "scratch_mounts",
        "action": "add",
        "key": input.target,
        "value": {"encrypted": input.encrypted},
    }
}

scratch_mount := {"metadata": [add_scratch_mount], "allowed": true} {
    not scratch_mounted(input.target)
    not data.policy.allow_unencrypted_scratch
    input.encrypted
    add_scratch_mount := {
        "name": "scratch_mounts",
        "action": "add",
        "key": input.target,
        "value": {"encrypted": input.encrypted},
    }
}

default scratch_unmount := {"allowed": false}

scratch_unmount := {"metadata": [remove_scratch_mount], "allowed": true} {
    scratch_mounted(input.unmountTarget)
    remove_scratch_mount := {
        "name": "scratch_mounts",
        "action": "remove",
        "key": input.unmountTarget,
    }
}

object_default(info, svn) := value {
    not info["item"]
    semver.compare(svn, info.introduced_version) >= 0
    value := null
}

object_default(info, svn) := value {
    not info["item"]
    semver.compare(svn, info.introduced_version) < 0
    value := info.default_value
}

item_default(info, svn) := value {
    semver.compare(svn, info.introduced_version) >= 0
    value := null
}

item_default(info, svn) := value {
    semver.compare(svn, info.introduced_version) < 0
    value := info.default_value
}

object_default(info, svn) := value {
    info["item"]
    value := {
        "__array__": true,
        "item": {key: value | value := item_default(info["item"][key], svn)}
    }
}

load_defaults(name, svn) := defaults {
    version_info := data.objectDefaults[name]
    defaults := {key: value | value := object_default(version_info[key], svn)}
}

# error messages

errors["deviceHash not found"] {
    input.rule == "mount_device"
    not deviceHash_ok
}

errors["device already mounted at path"] {
    input.rule == "mount_device"
    device_mounted(input.target)
}

errors["no device at path to unmount"] {
    input.rule == "unmount_device"
    not device_mounted(input.unmountTarget)
}

errors["container already started"] {
    input.rule == "create_container"
    container_started
}

errors["container not started"] {
    input.rule in ["exec_in_container", "shutdown_container", "signal_container_process"]
    not container_started
}

errors["overlay has already been mounted"] {
    input.rule == "mount_overlay"
    overlay_exists
}

default overlay_matches := false

overlay_matches {
    some container in data.policy.containers
    layerPaths_ok(container.layers)
}

overlay_matches {
    feed := data.metadata.issuers[_].feeds[_]
    some fragment in feed
    some container in fragment.containers
    layerPaths_ok(container.layers)
}

errors["no overlay at path to unmount"] {
    input.rule == "unmount_overlay"
    not overlay_mounted(input.unmountTarget)
}

errors["no matching containers for overlay"] {
    input.rule == "mount_overlay"
    not overlay_matches
}

default privileged_matches := false

privileged_matches {
    input.rule == "create_container"
    some container in data.metadata.matches[input.containerID]
    privileged_ok(container.allow_elevated)
}

errors["privileged escalation not allowed"] {
    input.rule in ["create_container"]
    not privileged_matches
}

default command_matches := false

command_matches {
    input.rule == "create_container"
    some container in data.metadata.matches[input.containerID]
    command_ok(container.command)
}

command_matches {
    input.rule == "exec_in_container"
    some container in data.metadata.matches[input.containerID]
    some process in container.exec_processes
    command_ok(process.command)
}

command_matches {
    input.rule == "exec_external"
    some process in data.policy.external_processes
    command_ok(process.command)
}

errors["invalid command"] {
    input.rule in ["create_container", "exec_in_container", "exec_external"]
    not command_matches
}

env_matches(env) {
    input.rule in ["create_container", "exec_in_container"]
    some container in data.metadata.matches[input.containerID]
    some rule in container.env_rules
    env_ok(rule.pattern, rule.strategy, env)
}

env_matches(env) {
    input.rule in ["exec_external"]
    some process in data.policy.external_processes
    some rule in process.env_rules
    env_ok(rule.pattern, rule.strategy, env)
}

errors[envError] {
    input.rule in ["create_container", "exec_in_container", "exec_external"]
    bad_envs := [invalid |
        env := input.envList[_]
        not env_matches(env)
        parts := split(env, "=")
        invalid = parts[0]
    ]

    count(bad_envs) > 0
    envError := concat(" ", ["invalid env list:", concat(",", bad_envs)])
}

default workingDirectory_matches := false

workingDirectory_matches {
    input.rule in ["create_container", "exec_in_container"]
    some container in data.metadata.matches[input.containerID]
    workingDirectory_ok(container.working_dir)
}

workingDirectory_matches {
    input.rule == "exec_external"
    some process in data.policy.external_processes
    workingDirectory_ok(process.working_dir)
}

errors["invalid working directory"] {
    input.rule in ["create_container", "exec_in_container", "exec_external"]
    not workingDirectory_matches
}

mount_matches(mount) {
    some container in data.metadata.matches[input.containerID]
    mount_ok(container.mounts, container.allow_elevated, mount)
}

errors[mountError] {
    input.rule == "create_container"
    bad_mounts := [mount.destination |
        mount := input.mounts[_]
        not mount_matches(mount)
    ]

    count(bad_mounts) > 0
    mountError := concat(" ", ["invalid mount list:", concat(",", bad_mounts)])
}

default signal_allowed := false

signal_allowed {
    some container in data.metadata.matches[input.containerID]
    signal_ok(container.signals)
}

signal_allowed {
    some container in data.metadata.matches[input.containerID]
    some process in container.exec_processes
    command_ok(process.command)
    signal_ok(process.signals)
}

errors["target isn't allowed to receive the signal"] {
    input.rule == "signal_container_process"
    not signal_allowed
}

errors["device already mounted at path"] {
    input.rule == "plan9_mount"
    plan9_mounted(input.target)
}

errors["no device at path to unmount"] {
    input.rule == "plan9_unmount"
    not plan9_mounted(input.unmountTarget)
}

default fragment_issuer_matches := false

fragment_issuer_matches {
    some fragment in data.policy.fragments
    fragment.issuer == input.issuer
}

fragment_issuer_matches {
    input.issuer in data.metadata.issuers
}

errors["invalid fragment issuer"] {
    input.rule == "load_fragment"
    not fragment_issuer_matches
}

default fragment_feed_matches := false

fragment_feed_matches {
    some fragment in data.policy.fragments
    fragment.issuer == input.issuer
    fragment.feed == input.feed
}

fragment_feed_matches {
    input.feed in data.metadata.issuers[input.issuer]
}

errors["invalid fragment feed"] {
    input.rule == "load_fragment"
    fragment_issuer_matches
    not fragment_feed_matches
}

default fragment_version_is_valid := false

fragment_version_is_valid {
    some fragment in data.policy.fragments
    fragment.issuer == input.issuer
    fragment.feed == input.feed
    semver.compare(data[input.namespace].svn, fragment.minimum_svn) >= 0
}

fragment_version_is_valid {
    some fragment in data.metadata.issuers[input.issuer][input.feed]
    semver.compare(data[input.namespace].svn, fragment.minimum_svn) >= 0
}

errors["fragment version is below the specified minimum"] {
    input.rule == "load_fragment"
    fragment_feed_matches
    not fragment_version_is_valid
}

errors["scratch already mounted at path"] {
    input.rule == "scratch_mount"
    scratch_mounted(input.target)
}

errors["unencrypted scratch not allowed"] {
    input.rule == "scratch_mount"
    not data.policy.allow_unencrypted_scratch
    not input.encrypted
}

errors["no scratch at path to unmount"] {
    input.rule == "scratch_unmount"
    not scratch_mounted(input.unmountTarget)
}

errors[framework_svn_error] {
    not data.policy.framework_svn
    framework_svn_error := concat(" ", ["framework_svn is missing. Current svn:", svn])
}

errors[framework_svn_error] {
    semver.compare(data.policy.framework_svn, svn) > 0
    framework_svn_error := concat(" ", ["framework_svn is ahead of the current svn:", data.policy.framework_svn, ">", svn])
}

errors[fragment_framework_svn_error] {
    not data[input.namespace].framework_svn
    fragment_framework_svn_error := concat(" ", ["fragment framework_svn is missing. Current svn:", svn])
}

errors[fragment_framework_svn_error] {
    semver.compare(data[input.namespace].framework_svn, svn) > 0
    fragment_framework_svn_error := concat(" ", ["fragment framework_svn is ahead of the current svn:", data[input.namespace].framework_svn, ">", svn])
}
