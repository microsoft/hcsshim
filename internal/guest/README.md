# Background
The packages in this directory comprise most of the code used to build the guest side agent for Linux Hyper-V containers on Windows (LCOW). The guest agent is designed to run inside a custom Linux OS for supporting Linux container payloads. It's a process that is designed to be connected to from a host machine to carry out requests for running containers in the Linux guest.

The two binaries of importance that are built from this code are the [main guest agent](../../cmd/gcs/main.go) that facilitates communication between the host and guest and a [generic tools binary](../../cmd/gcstools/main.go) for additional functionality for device scenarios.

As most of the rest of the repository is designed in part to run on Windows, the guest package tries to serve as a separation between the two. Any guest side LCOW specific feature work should end up here.