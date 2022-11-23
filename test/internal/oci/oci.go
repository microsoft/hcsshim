package oci

import (
	"context"
	"errors"
	"testing"

	"github.com/Microsoft/hcsshim/test/internal/constants"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	ctrdoci "github.com/containerd/containerd/oci"
	criconstants "github.com/containerd/containerd/pkg/cri/constants"
	"github.com/opencontainers/runtime-spec/specs-go"
)

//
// testing helper functions for OCI spec creation
//

const (
	TailNullArgs = "tail -f /dev/null"

	DefaultNamespace = namespaces.Default
	CRINamespace     = criconstants.K8sContainerdNamespace

	// from containerd\pkg\cri\server\helpers_linux.go

	HostnameEnv = "HOSTNAME"
)

func DefaultLinuxSpecOpts(nns string, extra ...ctrdoci.SpecOpts) []ctrdoci.SpecOpts {
	opts := []ctrdoci.SpecOpts{
		WithoutRunMount,
		ctrdoci.WithRootFSReadonly(),
		WithDisabledCgroups, // we set our own cgroups
		ctrdoci.WithDefaultUnixDevices,
		ctrdoci.WithDefaultPathEnv,
		WithWindowsNetworkNamespace(nns),
	}
	opts = append(opts, extra...)

	return opts
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
	return CreateSpecWithPlatform(ctx, tb, constants.PlatformLinux, id, opts...)
}

// CreateWindowsSpec returns the OCI spec for a Windows container.
//
// See [CreateSpecWithPlatform] for more details.
func CreateWindowsSpec(ctx context.Context, tb testing.TB, id string, opts ...ctrdoci.SpecOpts) *specs.Spec {
	tb.Helper()
	return CreateSpecWithPlatform(ctx, tb, constants.PlatformWindows, id, opts...)
}

// CreateSpecWithPlatform returns the OCI spec for the specified platform.
// The context must contain a containerd namespace added by
// [github.com/containerd/containerd/namespaces.WithNamespace]
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
	return func(ctx context.Context, client ctrdoci.Client, c *containers.Container, s *specs.Spec) error {
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

//defined in containerd\pkg\cri\opts\spec_windows.go

func WithWindowsNetworkNamespace(path string) ctrdoci.SpecOpts {
	return func(ctx context.Context, client ctrdoci.Client, c *containers.Container, s *specs.Spec) error {
		if s.Windows == nil {
			s.Windows = &specs.Windows{}
		}
		if s.Windows.Network == nil {
			s.Windows.Network = &specs.WindowsNetwork{}
		}
		s.Windows.Network.NetworkNamespace = path

		return nil
	}
}

//defined in containerd\pkg\cri\opts\spec_linux.go

// WithDisabledCgroups clears the Cgroups Path from the spec
func WithDisabledCgroups(_ context.Context, _ ctrdoci.Client, c *containers.Container, s *specs.Spec) error {
	if s.Linux == nil {
		s.Linux = &specs.Linux{}
	}
	s.Linux.CgroupsPath = ""
	return nil
}

// defined in containerd\oci\spec_opts_linux.go

// WithoutRunMount removes the `/run` inside the spec
func WithoutRunMount(ctx context.Context, client ctrdoci.Client, c *containers.Container, s *specs.Spec) error {
	return ctrdoci.WithoutMounts("/run")(ctx, client, c, s)
}
