//go:build linux

package gcs

// ContainerType values
// Following OCI annotations are used by katacontainers now.
// We'll switch to standard secure pod API after it is defined in CRI.
const (
	// ContainerTypeSandbox represents a pod sandbox container
	ContainerTypeSandbox = "sandbox"

	// ContainerTypeContainer represents a container running within a pod
	ContainerTypeContainer = "container"

	// ContainerType is the container type (sandbox or container) annotation
	ContainerType = "io.kubernetes.cri.container-type"

	// ContainerName is the name of the container in the pod
	ContainerName = "io.kubernetes.cri.container-name"

	// SandboxID is the sandbox ID annotation
	SandboxID = "io.kubernetes.cri.sandbox-id"

	// SandboxNamespace is the name of the namespace of the sandbox (pod)
	SandboxNamespace = "io.kubernetes.cri.sandbox-namespace"

	// SandboxName is the name of the sandbox (pod)
	SandboxName = "io.kubernetes.cri.sandbox-name"

	// SandboxLogDir is the pod log directory annotation.
	// If the sandbox needs to generate any log, it will put it into this directory.
	// Kubelet will be responsible for:
	// 1) Monitoring the disk usage of the log, and including it as part of the pod
	// ephemeral storage usage.
	// 2) Cleaning up the logs when the pod is deleted.
	// NOTE: Kubelet is not responsible for rotating the logs.
	SandboxLogDir = "io.kubernetes.cri.sandbox-log-directory"
)
