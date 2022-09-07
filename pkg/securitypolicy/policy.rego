package policy

api_svn := "0.1.0"

import future.keywords.every
import future.keywords.in

##OBJECTS##

default mount_device := false
mount_device := true {
    data.framework.mount_device
}

default mount_overlay := false
mount_overlay := true {
    data.framework.mount_overlay
}

default create_container := false
create_container := true {
    data.framework.create_container
}

reason := data.framework.reason
