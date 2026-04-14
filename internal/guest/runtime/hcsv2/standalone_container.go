//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"os"
	"path/filepath"

	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/guest/network"
	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func getStandaloneHostnamePath(rootDir string) string {
	return filepath.Join(rootDir, "hostname")
}

func getStandaloneHostsPath(rootDir string) string {
	return filepath.Join(rootDir, "hosts")
}

func getStandaloneResolvPath(rootDir string) string {
	return filepath.Join(rootDir, "resolv.conf")
}

func setupStandaloneContainerSpec(ctx context.Context, id, rootDir string, spec *oci.Spec) (err error) {
	ctx, span := oc.StartSpan(ctx, "hcsv2::setupStandaloneContainerSpec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", id))

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create container root directory %q", rootDir)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(rootDir)
		}
	}()

	hostname := spec.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return errors.Wrap(err, "failed to get hostname")
		}
	}

	// Write the hostname
	if !specGuest.MountPresent("/etc/hostname", spec.Mounts) {
		hostnamePath := getStandaloneHostnamePath(rootDir)
		if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0644); err != nil {
			return errors.Wrapf(err, "failed to write hostname to %q", hostnamePath)
		}

		mt := oci.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      hostnamePath,
			Options:     []string{"bind"},
		}
		if specGuest.IsRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Write the hosts
	if !specGuest.MountPresent("/etc/hosts", spec.Mounts) {
		hostsContent := network.GenerateEtcHostsContent(ctx, hostname)
		hostsPath := getStandaloneHostsPath(rootDir)
		if err := os.WriteFile(hostsPath, []byte(hostsContent), 0644); err != nil {
			return errors.Wrapf(err, "failed to write standalone hosts to %q", hostsPath)
		}

		mt := oci.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      hostsPath,
			Options:     []string{"bind"},
		}
		if specGuest.IsRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Write resolv.conf
	if !specGuest.MountPresent("/etc/resolv.conf", spec.Mounts) {
		ns := GetOrAddNetworkNamespace(specGuest.GetNetworkNamespaceID(spec))
		searches, servers := ns.dnsConfig(ctx)
		resolvContent, err := network.GenerateResolvConfContent(ctx, searches, servers, nil)
		if err != nil {
			return errors.Wrap(err, "failed to generate standalone resolv.conf content")
		}
		resolvPath := getStandaloneResolvPath(rootDir)
		if err := os.WriteFile(resolvPath, []byte(resolvContent), 0644); err != nil {
			return errors.Wrap(err, "failed to write standalone resolv.conf")
		}

		mt := oci.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      resolvPath,
			Options:     []string{"bind"},
		}
		if specGuest.IsRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Set cgroup path
	virtualSandboxID := spec.Annotations[annotations.VirtualPodID]
	if virtualSandboxID != "" {
		spec.Linux.CgroupsPath = "/containers/virtual-pods/" + virtualSandboxID + "/" + id
	} else {
		spec.Linux.CgroupsPath = "/containers/" + id
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
