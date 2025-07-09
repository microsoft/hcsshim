//go:build linux

package gcs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"

	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	runtime "k8s.io/cri-api/pkg/apis/runtime/v1"
)

//
// testing helper functions for generic container management
//

func sandboxSpec(
	ctx context.Context,
	tb testing.TB,
	name string,
	id string,
	nns string,
	root string,
	extra ...oci.SpecOpts,
) *oci.Spec {
	tb.Helper()
	ctx = namespaces.WithNamespace(ctx, testoci.CRINamespace)
	opts := sandboxSpecOpts(ctx, tb, name, id, nns, root)
	opts = append(opts, extra...)

	return testoci.CreateLinuxSpec(ctx, tb, id, opts...)
}

func sandboxSpecOpts(_ context.Context,
	tb testing.TB,
	name string,
	id string,
	nns string,
	root string,
) []oci.SpecOpts {
	tb.Helper()
	img := testoci.LinuxSandboxImageConfig(*flagSandboxPause)
	cfg := testoci.LinuxSandboxRuntimeConfig(name)

	opts := testoci.DefaultLinuxSpecOpts(nns,
		oci.WithEnv(img.Env),
		oci.WithHostname(cfg.GetHostname()),
		oci.WithRootFSPath(root),
	)

	if usr := img.User; usr != "" {
		oci.WithUser(usr)
	}

	if img.WorkingDir != "" {
		opts = append(opts, oci.WithProcessCwd(img.WorkingDir))
	}

	if len(img.Entrypoint) == 0 && len(img.Cmd) == 0 {
		tb.Fatalf("invalid empty entrypoint and cmd in image config %+v", img)
	}
	opts = append(opts, oci.WithProcessArgs(append(img.Entrypoint, img.Cmd...)...))

	opts = append(opts,
		withAnnotation(ContainerType, ContainerTypeSandbox),
		withAnnotation(SandboxID, id),
		withAnnotation(SandboxNamespace, cfg.GetMetadata().GetNamespace()),
		withAnnotation(SandboxName, cfg.GetMetadata().GetName()),
		withAnnotation(SandboxLogDir, cfg.GetLogDirectory()),
	)

	return opts
}

func containerSpec(
	ctx context.Context,
	tb testing.TB,
	sandboxID string,
	sandboxPID uint32,
	name string,
	id string,
	cmd []string,
	args []string,
	wd string,
	nns string,
	root string,
	extra ...oci.SpecOpts,
) *oci.Spec {
	tb.Helper()
	ctx = namespaces.WithNamespace(ctx, testoci.CRINamespace)
	opts := containerSpecOpts(ctx, tb, sandboxID, sandboxPID, name, cmd, args, wd, nns, root)
	opts = append(opts, extra...)

	return testoci.CreateLinuxSpec(ctx, tb, id, opts...)
}

func containerSpecOpts(_ context.Context, tb testing.TB,
	sandboxID string,
	sandboxPID uint32,
	name string,
	cmd []string,
	args []string,
	wd string,
	nns string,
	root string,
) []oci.SpecOpts {
	tb.Helper()
	cfg := testoci.LinuxWorkloadRuntimeConfig(name, cmd, args, wd)
	img := testoci.LinuxWorkloadImageConfig()

	opts := testoci.DefaultLinuxSpecOpts(nns,
		oci.WithRootFSPath(root),
		oci.WithEnv(nil),
		// this will be set based on the security context below
		oci.WithNewPrivileges,
		withProcessArgs(cfg, img),
		withPodNamespaces(nil, sandboxPID, sandboxPID, nil /* uids */, nil /* gids */),
	)

	hostname := name
	env := append([]string{testoci.HostnameEnv + "=" + hostname}, img.Env...)
	for _, e := range cfg.GetEnvs() {
		env = append(env, e.GetKey()+"="+e.GetValue())
	}
	opts = append(opts, oci.WithEnv(env))

	opts = append(opts,
		withAnnotation(ContainerType, ContainerTypeContainer),
		withAnnotation(SandboxID, sandboxID),
		withAnnotation(ContainerName, name),
	)

	return opts
}

func withAnnotation(k, v string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *runtimespec.Spec) error {
		if s.Annotations == nil {
			s.Annotations = make(map[string]string)
		}
		s.Annotations[k] = v
		return nil
	}
}

// WithProcessArgs sets the process args on the spec based on the image and runtime config
func withProcessArgs(config *runtime.ContainerConfig, image *imagespec.ImageConfig) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *runtimespec.Spec) (err error) {
		command, args := config.GetCommand(), config.GetArgs()
		// The following logic is migrated from https://github.com/moby/moby/blob/master/daemon/commit.go
		// TODO(random-liu): Clearly define the commands overwrite behavior.
		if len(command) == 0 {
			// Copy array to avoid data race.
			if len(args) == 0 {
				args = append([]string{}, image.Cmd...)
			}
			if command == nil {
				if len(image.Entrypoint) != 1 || image.Entrypoint[0] != "" {
					command = append([]string{}, image.Entrypoint...)
				}
			}
		}
		if len(command) == 0 && len(args) == 0 {
			return errors.New("no command specified")
		}
		return oci.WithProcessArgs(append(command, args...)...)(ctx, client, c, s)
	}
}

// withPodNamespaces sets the pod namespaces for the container
func withPodNamespaces(config *runtime.LinuxContainerSecurityContext, sandboxPid uint32, targetPid uint32, uids, gids []runtimespec.LinuxIDMapping) oci.SpecOpts {
	namespaces := config.GetNamespaceOptions()

	opts := []oci.SpecOpts{
		oci.WithLinuxNamespace(runtimespec.LinuxNamespace{Type: runtimespec.NetworkNamespace, Path: getNetworkNamespace(sandboxPid)}),
		oci.WithLinuxNamespace(runtimespec.LinuxNamespace{Type: runtimespec.IPCNamespace, Path: getIPCNamespace(sandboxPid)}),
		oci.WithLinuxNamespace(runtimespec.LinuxNamespace{Type: runtimespec.UTSNamespace, Path: getUTSNamespace(sandboxPid)}),
	}
	if namespaces.GetPid() != runtime.NamespaceMode_CONTAINER {
		opts = append(opts, oci.WithLinuxNamespace(runtimespec.LinuxNamespace{Type: runtimespec.PIDNamespace, Path: getPIDNamespace(targetPid)}))
	}

	if namespaces.GetUsernsOptions() != nil {
		switch namespaces.GetUsernsOptions().GetMode() {
		case runtime.NamespaceMode_NODE:
			// Nothing to do. Not adding userns field uses the node userns.
		case runtime.NamespaceMode_POD:
			opts = append(opts, oci.WithLinuxNamespace(runtimespec.LinuxNamespace{Type: runtimespec.UserNamespace, Path: getUserNamespace(sandboxPid)}))
			opts = append(opts, oci.WithUserNamespace(uids, gids))
		}
	}

	return oci.Compose(opts...)
}

const (
	// netNSFormat is the format of network namespace of a process.
	netNSFormat = "/proc/%v/ns/net"
	// ipcNSFormat is the format of ipc namespace of a process.
	ipcNSFormat = "/proc/%v/ns/ipc"
	// utsNSFormat is the format of uts namespace of a process.
	utsNSFormat = "/proc/%v/ns/uts"
	// pidNSFormat is the format of pid namespace of a process.
	pidNSFormat = "/proc/%v/ns/pid"
	// userNSFormat is the format of user namespace of a process.
	userNSFormat = "/proc/%v/ns/user"
)

// getNetworkNamespace returns the network namespace of a process.
func getNetworkNamespace(pid uint32) string {
	return fmt.Sprintf(netNSFormat, pid)
}

// getIPCNamespace returns the ipc namespace of a process.
func getIPCNamespace(pid uint32) string {
	return fmt.Sprintf(ipcNSFormat, pid)
}

// getPIDNamespace returns the uts namespace of a process.
func getUTSNamespace(pid uint32) string {
	return fmt.Sprintf(utsNSFormat, pid)
}

// getPIDNamespace returns the pid namespace of a process.
func getPIDNamespace(pid uint32) string {
	return fmt.Sprintf(pidNSFormat, pid)
}

// getUserNamespace returns the user namespace of a process.
func getUserNamespace(pid uint32) string {
	return fmt.Sprintf(userNSFormat, pid)
}
