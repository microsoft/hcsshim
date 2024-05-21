//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Microsoft/hcsshim/internal/guest/network"
	specInternal "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/otelutil"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func getSandboxHostnamePath(id string) string {
	return filepath.Join(specInternal.SandboxRootDir(id), "hostname")
}

func getSandboxHostsPath(id string) string {
	return filepath.Join(specInternal.SandboxRootDir(id), "hosts")
}

func getSandboxResolvPath(id string) string {
	return filepath.Join(specInternal.SandboxRootDir(id), "resolv.conf")
}

func setupSandboxContainerSpec(ctx context.Context, id string, spec *oci.Spec) (err error) {
	ctx, span := otelutil.StartSpan(ctx, "hcsv2::setupSandboxContainerSpec", trace.WithAttributes(
		attribute.String("cid", id)))
	defer span.End()
	defer func() { otelutil.SetSpanStatus(span, err) }()

	// Generate the sandbox root dir
	rootDir := specInternal.SandboxRootDir(id)
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandbox root directory %q", rootDir)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(rootDir)
		}
	}()

	// Write the hostname
	hostname := spec.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return errors.Wrap(err, "failed to get hostname")
		}
	}

	sandboxHostnamePath := getSandboxHostnamePath(id)
	if err := os.WriteFile(sandboxHostnamePath, []byte(hostname+"\n"), 0644); err != nil {
		return errors.Wrapf(err, "failed to write hostname to %q", sandboxHostnamePath)
	}

	// Write the hosts
	sandboxHostsContent := network.GenerateEtcHostsContent(ctx, hostname)
	sandboxHostsPath := getSandboxHostsPath(id)
	if err := os.WriteFile(sandboxHostsPath, []byte(sandboxHostsContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write sandbox hosts to %q", sandboxHostsPath)
	}

	// Write resolv.conf
	ns, err := getNetworkNamespace(getNetworkNamespaceID(spec))
	if err != nil {
		return err
	}
	var searches, servers []string
	for _, n := range ns.Adapters() {
		if len(n.DNSSuffix) > 0 {
			searches = network.MergeValues(searches, strings.Split(n.DNSSuffix, ","))
		}
		if len(n.DNSServerList) > 0 {
			servers = network.MergeValues(servers, strings.Split(n.DNSServerList, ","))
		}
	}
	resolvContent, err := network.GenerateResolvConfContent(ctx, searches, servers, nil)
	if err != nil {
		return errors.Wrap(err, "failed to generate sandbox resolv.conf content")
	}
	sandboxResolvPath := getSandboxResolvPath(id)
	if err := os.WriteFile(sandboxResolvPath, []byte(resolvContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write sandbox resolv.conf")
	}

	// User.Username is generally only used on Windows, but as there's no (easy/fast at least) way to grab
	// a uid:gid pairing for a username string on the host, we need to defer this work until we're here in the
	// guest. The username field is used as a temporary holding place until we can perform this work here when
	// we actually have the rootfs to inspect.
	if spec.Process.User.Username != "" {
		if err := setUserStr(spec, spec.Process.User.Username); err != nil {
			return err
		}
	}

	if rlimCore := spec.Annotations[annotations.RLimitCore]; rlimCore != "" {
		if err := setCoreRLimit(spec, rlimCore); err != nil {
			return err
		}
	}

	// TODO: JTERRY75 /dev/shm is not properly setup for LCOW I believe. CRI
	// also has a concept of a sandbox/shm file when the IPC NamespaceMode !=
	// NODE.

	// Force the parent cgroup into our /containers root
	spec.Linux.CgroupsPath = "/containers/" + id

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
