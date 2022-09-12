package policy

api_svn := "0.4.0"

import future.keywords.every
import future.keywords.in

##OBJECTS##

mount_device := data.framework.mount_device
unmount_device := data.framework.unmount_device
mount_overlay := data.framework.mount_overlay
create_container := data.framework.create_container
exec_in_container := data.framework.exec_in_container
exec_external := data.framework.exec_external
shutdown_container := data.framework.shutdown_container
reason := {"errors": data.framework.errors}
