package policy

import future.keywords.every
import future.keywords.in

default mount_device := false
mount_device := true {
    data.framework.mount_device
}

mount_device := true {
    count(data.policy.containers) == 0
    data.policy.allow_all
}

default mount_overlay := false
mount_overlay := true {
    data.framework.mount_overlay
}

mount_overlay := true {
    count(data.policy.containers) == 0
    data.policy.allow_all
}

default container_started := false
container_started := true {
    input.containerID in data.started
}

default create_container := false
create_container := true {
    data.framework.create_container
}


create_container := true {
    count(data.policy.containers) == 0
    data.policy.allow_all
}

default mount := false
mount := true {
    data.framework.mount
}

mount := true {
    count(data.policy.containers) == 0
    data.policy.allow_all
}

# error messages

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
