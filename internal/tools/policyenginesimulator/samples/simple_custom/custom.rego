package custom

import future.keywords.every
import future.keywords.in

default mount_device := {"allowed": false}

mount_device := {"allowed": true, "metadata": [addDevice] } {
    some overlay in data.policy.overlays
    some hash in overlay.deviceHashes
    hash == input.deviceHash
    addDevice := {
        "name": "devices",
        "action": "add",
        "key": input.target,
        "value": input.deviceHash
    }
}

default mount_overlay := {"allowed": false}

mount_overlay := {"allowed": true, "metadata": [addContainer]} {
    some overlayID, overlay in data.policy.overlays
    every i, path in input.layerPaths {
        hash := data.metadata.devices[path]
        hash == overlay.deviceHashes[i]
    }

    addContainer := {
        "name": "containers",
        "action": "add",
        "key": input.containerID,
        "value": overlayID
    }
}

default create_container := {"allowed": false}

create_container := {"allowed": true, "metadata": [updateContainer]} {
    overlayID := data.metadata.containers[input.containerID]
    overlay := data.policy.overlays[overlayID]
    some container in data.policy.custom_containers
    container.overlayID == overlayID
    every i, arg in input.argList {
        arg == container.command[i]
    }

    every mount in input.mounts {
        some destination in overlay.mounts
        mount.destination == destination
    }

    every depend in container.depends {
        some other in data.metadata.containers
        depend == other.id
    }

    updateContainer := {
        "name": "containers",
        "action": "update",
        "key": input.containerID,
        "value": container
    }
}