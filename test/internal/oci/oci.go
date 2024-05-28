package oci

import (
	"context"
	"errors"
	"testing"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	ctrdoci "github.com/containerd/containerd/oci"
	criconstants "github.com/containerd/containerd/pkg/cri/constants"
	criopts "github.com/containerd/containerd/pkg/cri/opts"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/Microsoft/hcsshim/pkg/annotations"

	"github.com/Microsoft/hcsshim/test/pkg/images"
)

//
// testing helper functions for OCI spec creation
//

const (
	TailNullArgs = "tail -f /dev/null"
	PingSelfCmd  = "cmd.exe /c ping -t 127.0.0.1"

	DefaultNamespace = namespaces.Default
	CRINamespace     = criconstants.K8sContainerdNamespace

	// from containerd\pkg\cri\server\helpers_linux.go

	HostnameEnv = "HOSTNAME"
)

func DefaultLinuxSpecOpts(nns string, extra ...ctrdoci.SpecOpts) []ctrdoci.SpecOpts {
	opts := []ctrdoci.SpecOpts{
		ctrdoci.WithoutRunMount,
		ctrdoci.WithRootFSReadonly(),
		criopts.WithDisabledCgroups, // we set our own cgroups
		ctrdoci.WithDefaultUnixDevices,
		ctrdoci.WithDefaultPathEnv,
		ctrdoci.WithWindowsNetworkNamespace(nns),
	}
	return append(opts, extra...)
}

func DefaultWindowsSpecOpts(nns string, extra ...ctrdoci.SpecOpts) []ctrdoci.SpecOpts {
	opts := []ctrdoci.SpecOpts{
		// make sure we set the Windows field
		func(_ context.Context, _ ctrdoci.Client, _ *containers.Container, s *specs.Spec) error {
			if s.Windows == nil {
				s.Windows = &specs.Windows{}
			}
			return nil
		},
		criopts.WithoutRoot,
		ctrdoci.WithProcessCwd(`C:\`),
		ctrdoci.WithWindowsNetworkNamespace(nns),
	}
	return append(opts, extra...)
}

// DefaultLinuxSpec returns a default OCI spec for a Linux container.
//
// See [CreateSpecWithPlatform] for more details.
func DefaultLinuxSpec(ctx context.Context, tb testing.TB, nns string) *specs.Spec {
	tb.Helper()
	return CreateLinuxSpec(ctx, tb, tb.Name(), DefaultLinuxSpecOpts(nns)...)
}

// CreateLinuxSpec returns the OCI spec for a Linux container.
//
// See [CreateSpecWithPlatform] for more details.
func CreateLinuxSpec(ctx context.Context, tb testing.TB, id string, opts ...ctrdoci.SpecOpts) *specs.Spec {
	tb.Helper()
	return CreateSpecWithPlatform(ctx, tb, images.PlatformLinux, id, opts...)
}

// CreateWindowsSpec returns the OCI spec for a Windows container.
//
// See [CreateSpecWithPlatform] for more details.
func CreateWindowsSpec(ctx context.Context, tb testing.TB, id string, opts ...ctrdoci.SpecOpts) *specs.Spec {
	tb.Helper()
	return CreateSpecWithPlatform(ctx, tb, images.PlatformWindows, id, opts...)
}

// CreateSpecWithPlatform returns the OCI spec for the specified platform.
// The context must contain a containerd namespace added by
// [github.com/containerd/containerd/namespaces.WithNamespace].
func CreateSpecWithPlatform(ctx context.Context, tb testing.TB, plat, id string, opts ...ctrdoci.SpecOpts) *specs.Spec {
	tb.Helper()
	container := &containers.Container{ID: id}

	spec, err := ctrdoci.GenerateSpecWithPlatform(ctx, nil, plat, container, opts...)
	if err != nil {
		tb.Fatalf("failed to generate spec for container %q: %v", id, err)
	}

	return spec
}

func WithWindowsLayerFolders(layers []string) ctrdoci.SpecOpts {
	return func(_ context.Context, _ ctrdoci.Client, _ *containers.Container, s *specs.Spec) error {
		if len(layers) < 2 {
			return errors.New("at least two layers are required, including the sandbox path")
		}

		if s.Windows == nil {
			s.Windows = &specs.Windows{}
		}
		s.Windows.LayerFolders = layers

		return nil
	}
}

// AsHostProcessContainer updates the spec to create a HostProcess container.
func AsHostProcessContainer() ctrdoci.SpecOpts {
	return func(_ context.Context, _ ctrdoci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Annotations == nil {
			s.Annotations = make(map[string]string)
		}
		s.Annotations[annotations.HostProcessContainer] = "true"
		return nil
	}
}

// HostProcessInheritUser updates the spec to allow the HostProcess container to inherit the current
// user's token.
func HostProcessInheritUser() ctrdoci.SpecOpts {
	return func(_ context.Context, _ ctrdoci.Client, _ *containers.Container, s *specs.Spec) error {
		if s.Annotations == nil {
			s.Annotations = make(map[string]string)
		}
		s.Annotations[annotations.HostProcessInheritUser] = "true"
		return nil
	}
}
