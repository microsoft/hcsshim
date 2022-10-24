package framework

import future.keywords.every
import future.keywords.in

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

mount_device := {"devices": devices, "allowed": true} {
    not device_mounted(input.target)
    deviceHash_ok
    devices := {
        "action": "add",
        "key": input.target,
        "value": input.deviceHash
    }
}

default unmount_device := {"allowed": false}

unmount_device := {"devices": devices, "allowed": true} {
    device_mounted(input.unmountTarget)
    devices := {
        "action": "remove",
        "key": input.unmountTarget
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

default mount_overlay := {"allowed": false}

mount_overlay := {"matches": matches, "overlayTargets": overlay_targets, "allowed": true} {
    not overlay_exists
    # we need to assemble a list of all possible containers
    # which match the overlay requested, including both
    # containers in the policy and those included from fragments.
    policy_containers := [container |
        container := data.policy.containers[_]
        layerPaths_ok(container.layers)
    ]
    fragment_containers := [container |
        feed := data.metadata.issuers[_].feeds[_]
        some fragment in feed
        container := fragment.containers[_]
        layerPaths_ok(container.layers)
    ]
    containers := array.concat(policy_containers, fragment_containers)
    count(containers) > 0
    matches := {
        "action": "add",
        "key": input.containerID,
        "value": containers
    }
    overlay_targets := {
        "action": "add",
        "key": input.target,
        "value": true
    }
}

default unmount_overlay := {"allowed": false}

unmount_overlay := {"overlayTargets": overlay_targets, "allowed": true} {
    overlay_mounted(input.unmountTarget)
    overlay_targets := {
        "action": "remove",
        "key": input.unmountTarget
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

envList_ok(env_rules) {
    every env in input.envList {
        some rule in env_rules
        env_ok(rule.pattern, rule.strategy, env)
    }

    every rule in env_rules {
        some env in input.envList
        rule_ok(rule, env)
    }
}

workingDirectory_ok(working_dir) {
    input.workingDir == working_dir
}

default container_started := false

container_started {
    data.metadata.started[input.containerID]
}

default create_container := {"allowed": false}

create_container := {"matches": matches, "started": started, "allowed": true} {
    not container_started
    containers := [container |
        container := data.metadata.matches[input.containerID][_]
        command_ok(container.command)
        envList_ok(container.env_rules)
        workingDirectory_ok(container.working_dir)
        mountList_ok(container.mounts, container.allow_elevated)
    ]
    count(containers) > 0
    matches := {
        "action": "update",
        "key": input.containerID,
        "value": containers
    }
    started := {
        "action": "add",
        "key": input.containerID,
        "value": true
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

exec_in_container := {"matches": matches, "allowed": true} {
    container_started
    containers := [container |
        container := data.metadata.matches[input.containerID][_]
        envList_ok(container.env_rules)
        workingDirectory_ok(container.working_dir)
        some process in container.exec_processes
        command_ok(process.command)
    ]
    count(containers) > 0
    matches := {
        "action": "update",
        "key": input.containerID,
        "value": containers
    }
}

default shutdown_container := {"allowed": false}

shutdown_container := {"started": remove, "matches": remove, "allowed": true} {
    container_started
    remove := {
        "action": "remove",
        "key": input.containerID,
    }
}

default signal_container_process := {"allowed": false}

signal_container_process := {"matches": matches, "allowed": true} {
    container_started
    input.isInitProcess
    containers := [container |
        container := data.metadata.matches[input.containerID][_]
        signal_ok(container.signals)
    ]
    count(containers) > 0
    matches := {
        "action": "update",
        "key": input.containerID,
        "value": containers
    }
}

signal_container_process := {"matches": matches, "allowed": true} {
    container_started
    not input.isInitProcess
    containers := [container |
        container := data.metadata.matches[input.containerID][_]
        some process in container.exec_processes
        command_ok(process.command)
        signal_ok(process.signals)
    ]
    count(containers) > 0
    matches := {
        "action": "update",
        "key": input.containerID,
        "value": containers
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

plan9_mount := {"p9mounts": p9mounts, "allowed": true} {
    not plan9_mounted(input.target)
    some containerID, _ in data.metadata.matches
    pattern := concat("", [input.rootPrefix, "/", containerID, input.mountPathPrefix])
    regex.match(pattern, input.target)
    p9mounts := {
        "action": "add",
        "key": input.target,
        "value": containerID
    }
}

default plan9_unmount := {"allowed": false}

plan9_unmount := {"p9mounts": p9mounts, "allowed": true} {
    plan9_mounted(input.target)
    p9mounts := {
        "action": "remove",
        "key": input.target,
    }
}


default enforcement_point_info := {"available": false, "allowed": false, "unknown": true, "invalid": false}

enforcement_point_info := {"available": available, "allowed": allowed, "unknown": false, "invalid": false} {
    enforcement_point := data.api.enforcement_points[input.name]
    semver.compare(data.api.svn, enforcement_point.introducedVersion) >= 0
    available := semver.compare(data.policy.api_svn, enforcement_point.introducedVersion) >= 0
    allowed := enforcement_point.allowedByDefault
}

enforcement_point_info := {"available": false, "allowed": false, "unknown": false, "invalid": true} {
    enforcement_point := data.api.enforcement_points[input.name]
    semver.compare(data.api.svn, enforcement_point.introducedVersion) < 0
}

external_process_ok(process) {
    command_ok(process.command)
    envList_ok(process.env_rules)
    workingDirectory_ok(process.working_dir)
}

default exec_external := {"allowed": false}

# test if there is a matching external process in the policy
exec_external := {"allowed": true} {
    some process in data.policy.external_processes
    external_process_ok(process)
}

# test if there is a matching external process in a fragment
exec_external := {"allowed": true} {
    feed := data.metadata.issuers[_].feeds[_]
    some fragment in feed
    some process in fragment.external_processes
    external_process_ok(process)
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

extract_fragment_includes(includes) := fragment {
    objects := {
        "containers": fragment_containers,
        "fragments": fragment_fragments,
        "external_processes": fragment_external_processes
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
    new_issuer := {
        "feeds": {
            input.feed: array.concat([extract_fragment_includes(includes)], old_fragments)
        }
    }
    issuer := object.union(old_issuer, new_issuer)
}

update_issuer(includes) := issuer {
    not feed_exists(input.issuer, input.feed)
    old_issuer := data.metadata.issuers[input.issuer]
    new_issuer := {
        "feeds": {
            input.feed: [extract_fragment_includes(includes)]
        }
    }
    issuer := object.union(old_issuer, new_issuer)
}

update_issuer(includes) := issuer {
    not issuer_exists(input.issuer)
    issuer := {
        "feeds": {
            input.feed: [extract_fragment_includes(includes)]
        }
    }
}

default load_fragment := {"allowed": false}

fragment_ok(fragment) {
    input.issuer == fragment.issuer
    input.feed == fragment.feed
    semver.compare(data[input.namespace].svn, fragment.minimum_svn) >= 0
}


# test if there is a matching fragment in the policy
matching_fragment := fragment {
    some fragment in data.policy.fragments
    fragment_ok(fragment)
}

# test if there is a matching fragment in a fragment
matching_fragment := subfragment {
    feed := data.metadata.issuers[_].feeds[_]
    some fragment in feed
    some subfragment in fragment.fragments
    fragment_ok(subfragment)
}

load_fragment := {"issuers": issuers, "add_module": add_module, "allowed": true} {
    fragment := matching_fragment
    issuer := update_issuer(fragment.includes)
    issuers := {
        "action": "update",
        "key": input.issuer,
        "value": issuer
    }

    add_module := "namespace" in fragment.includes
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

default envList_matches := false

envList_matches {
    input.rule in ["create_container", "exec_in_container"]
    some container in data.metadata.matches[input.containerID]
    envList_ok(container.env_rules)
}

envList_matches {
    input.rule == "exec_external"
    some process in data.policy.external_processes
    envList_ok(process.env_rules)
}

errors["invalid env list"] {
    input.rule in ["create_container", "exec_in_container", "exec_external"]
    not envList_matches
}

default workingDirectory_matches := false

workingDirectory_matches {
    input.rule in ["create_container", "exec_in_container"]
    some container in data.metadata.matches[input.containerID]
    workingDirectory_ok(container.working_dir)
}

workingDirectory_matches {
    input.rule == "exec_external"
    some process in data.external_processes
    workingDirectory_ok(process.working_dir)
}

errors["invalid working directory"] {
    input.rule in ["create_container", "exec_in_container", "exec_external"]
    not workingDirectory_matches
}

default mountList_matches := false

mountList_matches {
    some container in data.metadata.matches[input.containerID]
    data.framework.mountList_ok(container, container.allow_elevated)
}

errors["invalid mount list"] {
    input.rule == "create_container"
    not mountList_matches
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
