//go:build linux

package gcs

import (
	"context"
	"testing"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/pkg/cri/annotations"
	criopts "github.com/containerd/containerd/pkg/cri/opts"

	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
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
		criopts.WithAnnotation(annotations.ContainerType, annotations.ContainerTypeSandbox),
		criopts.WithAnnotation(annotations.SandboxID, id),
		criopts.WithAnnotation(annotations.SandboxNamespace, cfg.GetMetadata().GetNamespace()),
		criopts.WithAnnotation(annotations.SandboxName, cfg.GetMetadata().GetName()),
		criopts.WithAnnotation(annotations.SandboxLogDir, cfg.GetLogDirectory()),
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
		criopts.WithProcessArgs(cfg, img),
		criopts.WithPodNamespaces(nil, sandboxPID, sandboxPID, nil /* uids */, nil /* gids */),
	)

	hostname := name
	env := append([]string{testoci.HostnameEnv + "=" + hostname}, img.Env...)
	for _, e := range cfg.GetEnvs() {
		env = append(env, e.GetKey()+"="+e.GetValue())
	}
	opts = append(opts, oci.WithEnv(env))

	opts = append(opts,
		criopts.WithAnnotation(annotations.ContainerType, annotations.ContainerTypeContainer),
		criopts.WithAnnotation(annotations.SandboxID, sandboxID),
		criopts.WithAnnotation(annotations.ContainerName, name),
	)

	return opts
}
