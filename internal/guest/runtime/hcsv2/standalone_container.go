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
	"go.opencensus.io/trace"

	"github.com/Microsoft/hcsshim/internal/guest/network"
	specGuest "github.com/Microsoft/hcsshim/internal/guest/spec"
	"github.com/Microsoft/hcsshim/internal/guestpath"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func getStandaloneRootDir(id, virtualSandboxID string) string {
	if virtualSandboxID != "" {
		// Standalone container in virtual pod gets its own subdir
		return filepath.Join(guestpath.LCOWRootPrefixInUVM, "virtual-pods", virtualSandboxID, id)
	}
	return filepath.Join(guestpath.LCOWRootPrefixInUVM, id)
}

func getStandaloneHostnamePath(id, virtualSandboxID string) string {
	return filepath.Join(getStandaloneRootDir(id, virtualSandboxID), "hostname")
}

func getStandaloneHostsPath(id, virtualSandboxID string) string {
	return filepath.Join(getStandaloneRootDir(id, virtualSandboxID), "hosts")
}

func getStandaloneResolvPath(id, virtualSandboxID string) string {
	return filepath.Join(getStandaloneRootDir(id, virtualSandboxID), "resolv.conf")
}

func setupStandaloneContainerSpec(ctx context.Context, id string, spec *oci.Spec) (err error) {
	ctx, span := oc.StartSpan(ctx, "hcsv2::setupStandaloneContainerSpec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", id))

	// Check if this is a virtual pod (unlikely for standalone)
	virtualSandboxID := spec.Annotations[annotations.VirtualPodID]

	// Generate the standalone root dir - virtual pod aware
	rootDir := getStandaloneRootDir(id, virtualSandboxID)
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
		standaloneHostnamePath := getStandaloneHostnamePath(id, virtualSandboxID)
		if err := os.WriteFile(standaloneHostnamePath, []byte(hostname+"\n"), 0644); err != nil {
			return errors.Wrapf(err, "failed to write hostname to %q", standaloneHostnamePath)
		}

		mt := oci.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      getStandaloneHostnamePath(id, virtualSandboxID),
			Options:     []string{"bind"},
		}
		if specGuest.IsRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Write the hosts
	if !specGuest.MountPresent("/etc/hosts", spec.Mounts) {
		standaloneHostsContent := network.GenerateEtcHostsContent(ctx, hostname)
		standaloneHostsPath := getStandaloneHostsPath(id, virtualSandboxID)
		if err := os.WriteFile(standaloneHostsPath, []byte(standaloneHostsContent), 0644); err != nil {
			return errors.Wrapf(err, "failed to write standalone hosts to %q", standaloneHostsPath)
		}

		mt := oci.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      getStandaloneHostsPath(id, virtualSandboxID),
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
			return errors.Wrap(err, "failed to generate standalone resolv.conf content")
		}
		standaloneResolvPath := getStandaloneResolvPath(id, virtualSandboxID)
		if err := os.WriteFile(standaloneResolvPath, []byte(resolvContent), 0644); err != nil {
			return errors.Wrap(err, "failed to write standalone resolv.conf")
		}

		mt := oci.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      getStandaloneResolvPath(id, virtualSandboxID),
			Options:     []string{"bind"},
		}
		if specGuest.IsRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Set cgroup path - check if this is part of a virtual pod (unlikely for standalone)
	if virtualSandboxID != "" {
		// Standalone container in virtual pod goes under /containers/virtual-pods/{virtualSandboxID}/{containerID}
		// Each virtualSandboxID creates its own pod-level cgroup for all containers in that virtual pod
		spec.Linux.CgroupsPath = "/containers/virtual-pods/" + virtualSandboxID + "/" + id
	} else {
		// Traditional standalone container goes under /containers
		spec.Linux.CgroupsPath = "/containers/" + id
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
