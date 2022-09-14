package framework

import future.keywords.every
import future.keywords.in

device_mounted(target) {
    data.metadata.devices[target]
}

default mount_device := {"allowed": false}

mount_device := {"devices": devices, "allowed": true} {
    not device_mounted(input.target)
    some container in data.policy.containers
    some layer in container.layers
    input.deviceHash == layer
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

default mount_overlay := {"allowed": false}

mount_overlay := {"matches": matches, "allowed": true} {
    not overlay_exists
    containers := [container |
        some container in data.policy.containers
        layerPaths_ok(container.layers)
    ]
    count(containers) > 0
    matches := {
        "action": "add",
        "key": input.containerID,
        "value": containers
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
        some container in data.metadata.matches[input.containerID]
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
        some container in data.metadata.matches[input.containerID]
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

# error messages

errors["container already started"] {
    input.rule == "create_container"
    container_started
}

errors["container not started"] {
    input.rule == "exec_in_container"
    not container_started
}

default command_matches := false

command_matches {
    input.rule == "create_container"
    some container in data.metadata.matches[input.containerID]
    data.framework.command_ok(container.command)
}

command_matches {
    input.rule == "exec_in_container"
    some container in data.metadata.matches[input.containerID]
    some process in container.exec_processes
    data.framework.command_ok(process.command)
}

errors["invalid command"] {
    not command_matches
}

default envList_matches := false

envList_matches {
    input.rule == "create_container"
    some container in data.metadata.matches[input.containerID]
    data.framework.envList_ok(container.env_rules)
}

envList_matches {
    input.rule == "exec_in_container"
    some container in data.metadata.matches[input.containerID]
    some process in container.exec_processes
    data.framework.envList_ok(container.env_rules)
}

errors["invalid env list"] {
    not envList_matches
}

default workingDirectory_matches := false

workingDirectory_matches {
    input.rule == "create_container"
    some container in data.metadata.matches[input.containerID]
    data.framework.workingDirectory_ok(container.working_dir)
}

workingDirectory_matches {
    input.rule == "exec_in_container"
    some container in data.metadata.matches[input.containerID]
    some process in container.exec_processes
    data.framework.workingDirectory_ok(container.working_dir)
}

errors["invalid working directory"] {
    not workingDirectory_matches
}

default mountList_matches := false

mountList_matches {
    input.rule == "create_container"
    some container in data.metadata.matches[input.containerID]
    data.framework.mountList_ok(container, container.allow_elevated)
}

errors["invalid mount list"] {
    not mountList_matches
}
