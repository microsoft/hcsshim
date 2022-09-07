//go:build linux
// +build linux

package network

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/storage"
	"github.com/Microsoft/hcsshim/internal/guest/storage/pci"
	"github.com/Microsoft/hcsshim/internal/guest/storage/vmbus"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// mock out calls for testing
var (
	pciFindDeviceFullPath             = pci.FindDeviceFullPath
	storageWaitForFileMatchingPattern = storage.WaitForFileMatchingPattern
	vmbusWaitForDevicePath            = vmbus.WaitForDevicePath
	ioReadDir                         = os.ReadDir
)

// maxDNSSearches is limited to 6 in `man 5 resolv.conf`
const maxDNSSearches = 6

// GenerateEtcHostsContent generates a /etc/hosts file based on `hostname`.
func GenerateEtcHostsContent(ctx context.Context, hostname string) string {
	_, span := oc.StartSpan(ctx, "network::GenerateEtcHostsContent")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("hostname", hostname))

	nameParts := strings.Split(hostname, ".")
	buf := bytes.Buffer{}
	buf.WriteString("127.0.0.1 localhost\n")
	if len(nameParts) > 1 {
		buf.WriteString(fmt.Sprintf("127.0.0.1 %s %s\n", hostname, nameParts[0]))
	} else {
		buf.WriteString(fmt.Sprintf("127.0.0.1 %s\n", hostname))
	}
	buf.WriteString("\n")
	buf.WriteString("# The following lines are desirable for IPv6 capable hosts\n")
	buf.WriteString("::1     ip6-localhost ip6-loopback\n")
	buf.WriteString("fe00::0 ip6-localnet\n")
	buf.WriteString("ff00::0 ip6-mcastprefix\n")
	buf.WriteString("ff02::1 ip6-allnodes\n")
	buf.WriteString("ff02::2 ip6-allrouters\n")
	return buf.String()
}

// GenerateResolvConfContent generates the resolv.conf file content based on
// `searches`, `servers`, and `options`.
func GenerateResolvConfContent(ctx context.Context, searches, servers, options []string) (_ string, err error) {
	_, span := oc.StartSpan(ctx, "network::GenerateResolvConfContent")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("searches", strings.Join(searches, ", ")),
		trace.StringAttribute("servers", strings.Join(servers, ", ")),
		trace.StringAttribute("options", strings.Join(options, ", ")))

	if len(searches) > maxDNSSearches {
		return "", errors.Errorf("searches has more than %d domains", maxDNSSearches)
	}

	content := ""
	if len(searches) > 0 {
		content += fmt.Sprintf("search %s\n", strings.Join(searches, " "))
	}
	if len(servers) > 0 {
		content += fmt.Sprintf("nameserver %s\n", strings.Join(servers, "\nnameserver "))
	}
	if len(options) > 0 {
		content += fmt.Sprintf("options %s\n", strings.Join(options, " "))
	}
	return content, nil
}

// MergeValues merges `first` and `second` maintaining order `first, second`.
func MergeValues(first, second []string) []string {
	if len(first) == 0 {
		return second
	}
	if len(second) == 0 {
		return first
	}
	values := make([]string, len(first), len(first)+len(second))
	copy(values, first)
	for _, v := range second {
		found := false
		for i := 0; i < len(values); i++ {
			if v == values[i] {
				found = true
				break
			}
		}
		if !found {
			values = append(values, v)
		}
	}
	return values
}

// InstanceIDToName converts from the given instance ID (a GUID generated on the
// Windows host) to its corresponding interface name (e.g. "eth0").
//
// Will retry the operation until `ctx` is exceeded or canceled.
func InstanceIDToName(ctx context.Context, id string, vpciAssigned bool) (_ string, err error) {
	ctx, span := oc.StartSpan(ctx, "network::InstanceIDToName")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	vmBusID := strings.ToLower(id)
	span.AddAttributes(trace.StringAttribute("adapterInstanceID", vmBusID))

	var netDevicePath string
	if vpciAssigned {
		var pciDevicePath string
		pciDevicePath, err = pciFindDeviceFullPath(ctx, vmBusID)
		if err != nil {
			return "", err
		}
		pciNetDirPattern := filepath.Join(pciDevicePath, "net")
		netDevicePath, err = storageWaitForFileMatchingPattern(ctx, pciNetDirPattern)
	} else {
		vmBusNetSubPath := filepath.Join(vmBusID, "net")
		netDevicePath, err = vmbusWaitForDevicePath(ctx, vmBusNetSubPath)
	}
	if err != nil {
		return "", errors.Wrapf(err, "failed to find adapter %v sysfs path", vmBusID)
	}

	var deviceDirs []os.DirEntry
	for {
		deviceDirs, err = ioReadDir(netDevicePath)
		if err != nil {
			if os.IsNotExist(err) {
				select {
				case <-ctx.Done():
					return "", errors.Wrap(ctx.Err(), "timed out waiting for net adapter")
				default:
					time.Sleep(10 * time.Millisecond)
					continue
				}
			} else {
				return "", errors.Wrapf(err, "failed to read vmbus network device from /sys filesystem for adapter %s", vmBusID)
			}
		}
		break
	}
	if len(deviceDirs) == 0 {
		return "", errors.Errorf("no interface name found for adapter %s", vmBusID)
	}
	if len(deviceDirs) > 1 {
		return "", errors.Errorf("multiple interface names found for adapter %s", vmBusID)
	}
	ifname := deviceDirs[0].Name()
	log.G(ctx).WithField("ifname", ifname).Debug("resolved ifname")
	return ifname, nil
}
