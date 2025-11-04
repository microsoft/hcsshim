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
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/pkg/annotations"
)

func getSandboxHostnamePath(id, virtualSandboxID string) string {
	return filepath.Join(specGuest.VirtualPodAwareSandboxRootDir(id, virtualSandboxID), "hostname")
}

func getSandboxHostsPath(id, virtualSandboxID string) string {
	return filepath.Join(specGuest.VirtualPodAwareSandboxRootDir(id, virtualSandboxID), "hosts")
}

func getSandboxResolvPath(id, virtualSandboxID string) string {
	return filepath.Join(specGuest.VirtualPodAwareSandboxRootDir(id, virtualSandboxID), "resolv.conf")
}

func setupSandboxContainerSpec(ctx context.Context, id string, spec *oci.Spec) (err error) {
	ctx, span := oc.StartSpan(ctx, "hcsv2::setupSandboxContainerSpec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", id))

	// Check if this is a virtual pod to use appropriate root directory
	virtualSandboxID := spec.Annotations[annotations.VirtualPodID]

	// Generate the sandbox root dir - virtual pod aware
	rootDir := specGuest.VirtualPodAwareSandboxRootDir(id, virtualSandboxID)
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

	sandboxHostnamePath := getSandboxHostnamePath(id, virtualSandboxID)
	if err := os.WriteFile(sandboxHostnamePath, []byte(hostname+"\n"), 0644); err != nil {
		return errors.Wrapf(err, "failed to write hostname to %q", sandboxHostnamePath)
	}

	// Write the hosts
	sandboxHostsContent := network.GenerateEtcHostsContent(ctx, hostname)
	sandboxHostsPath := getSandboxHostsPath(id, virtualSandboxID)
	if err := os.WriteFile(sandboxHostsPath, []byte(sandboxHostsContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write sandbox hosts to %q", sandboxHostsPath)
	}

	// Check if this is a virtual pod sandbox container by comparing container ID with virtual pod ID
	isVirtualPodSandbox := virtualSandboxID != "" && id == virtualSandboxID
	if strings.EqualFold(spec.Annotations[annotations.SkipPodNetworking], "true") || isVirtualPodSandbox {
		ns := GetOrAddNetworkNamespace(specGuest.GetNetworkNamespaceID(spec))
		err := ns.Sync(ctx)
		if err != nil {
			return err
		}
	}
	// Write resolv.conf
	ns, err := getNetworkNamespace(specGuest.GetNetworkNamespaceID(spec))
	if err != nil {
		if !strings.EqualFold(spec.Annotations[annotations.SkipPodNetworking], "true") {
			return err
		}
		// Networking is skipped, do not error out
		log.G(ctx).Infof("setupSandboxContainerSpec: Did not find NS spec %v, err %v", spec, err)
	} else {
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
		sandboxResolvPath := getSandboxResolvPath(id, virtualSandboxID)
		if err := os.WriteFile(sandboxResolvPath, []byte(resolvContent), 0644); err != nil {
			return errors.Wrap(err, "failed to write sandbox resolv.conf")
		}
	}

	// User.Username is generally only used on Windows, but as there's no (easy/fast at least) way to grab
	// a uid:gid pairing for a username string on the host, we need to defer this work until we're here in the
	// guest. The username field is used as a temporary holding place until we can perform this work here when
	// we actually have the rootfs to inspect.
	if spec.Process.User.Username != "" {
		if err := specGuest.SetUserStr(spec, spec.Process.User.Username); err != nil {
			return err
		}
	}

	if rlimCore := spec.Annotations[annotations.RLimitCore]; rlimCore != "" {
		if err := specGuest.SetCoreRLimit(spec, rlimCore); err != nil {
			return err
		}
	}

	// TODO: JTERRY75 /dev/shm is not properly setup for LCOW I believe. CRI
	// also has a concept of a sandbox/shm file when the IPC NamespaceMode !=
	// NODE.

	// Set cgroup path - check if this is a virtual pod
	if virtualSandboxID != "" {
		// Virtual pod sandbox gets its own cgroup under /containers/virtual-pods using the virtual pod ID
		spec.Linux.CgroupsPath = "/containers/virtual-pods/" + virtualSandboxID
	} else {
		// Traditional sandbox goes under /containers
		spec.Linux.CgroupsPath = "/containers/" + id
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
