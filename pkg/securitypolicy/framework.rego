package framework

import future.keywords.every
import future.keywords.in

default mount_device := false
mount_device := true {
    some container in data.policy.containers
    some layer in container.layers
    input.deviceHash == layer
}

layerPaths_ok(container) {
    length := count(container.layers)
    count(input.layerPaths) == length
    every i, path in input.layerPaths {
        container.layers[length - i - 1] == data.devices[path]
    }
}

default mount_overlay := false
mount_overlay := true {
    some container in data.policy.containers
    layerPaths_ok(container)
}

command_ok(container) {
    count(input.argList) == count(container.command)
    every i, arg in input.argList {
        container.command[i] == arg
    }
}

env_ok(pattern, "string", value) {
    pattern == value
}

env_ok(pattern, "re2", value) {
    regex.match(pattern, value)
}

envList_ok(container) {
    every env in input.envList {
        some rule in container.env_rules
        env_ok(rule.pattern, rule.strategy, env)
    }
}

workingDirectory_ok(container) {
    input.workingDir == container.working_dir
}

default create_container := false
create_container := true {
    not input.containerID in data.started
    some container in data.policy.containers
    layerPaths_ok(container)
    command_ok(container)
    envList_ok(container)
    workingDirectory_ok(container)
    mountList_ok(container)
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

mount_ok(container, mount) {
    some constraint in container.mounts
    mountConstraint_ok(constraint, mount)
}

mount_ok(container, mount) {
    some constraint in data.defaultMounts
    mountConstraint_ok(constraint, mount)
}

mount_ok(container, mount) {
    container.allow_elevated
    some constraint in data.privilegedMounts
    mountConstraint_ok(constraint, mount)
}

mountList_ok(container) {
    every mount in input.mounts {
        mount_ok(container, mount)
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

default container_started := false
container_started := true {
    input.containerID in data.started
}

reason["container already started"] {
    input.rule == "create_container"
    container_started
}

default command_matches := false
command_matches := true {
    some container in data.policy.containers
    data.framework.command_ok(container)
}

reason["invalid command"] {
    not command_matches
}

default envList_matches := false
envList_matches := true {
    some container in data.policy.containers
    data.framework.envList_ok(container)
}

reason["invalid env list"] {
    not envList_matches
}

default workingDirectory_matches := false
workingDirectory_matches := true {
    some container in data.policy.containers
    data.framework.workingDirectory_ok(container)
}

reason["invalid working directory"] {
    not workingDirectory_matches
}

default mountList_matches := false
mountList_matches := true {
    some container in data.policy.containers
    data.framework.mountList_ok(container)
}

reason["invalid mount list"] {
    not mountList_matches
}
