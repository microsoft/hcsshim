package gcs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"github.com/Microsoft/opengcs/service/gcs/oslayer"
	"github.com/Microsoft/opengcs/service/gcs/prot"
)

// instanceIDToName converts from the given instance ID (a GUID generated on
// the Windows host) to its corresponding interface name (e.g. "eth0").
func (c *gcsCore) instanceIDToName(id string) (string, error) {
	deviceDirs, err := c.OS.ReadDir(filepath.Join("/sys", "bus", "vmbus", "devices", id, "net"))
	if err != nil {
		return "", errors.Wrapf(err, "failed to read vmbus network device from /sys filesystem for adapter %s", id)
	}
	if len(deviceDirs) == 0 {
		return "", errors.Errorf("no interface name found for adapter %s", id)
	}
	if len(deviceDirs) > 1 {
		return "", errors.Errorf("multiple interface names found for adapter %s", id)
	}
	return deviceDirs[0].Name(), nil
}

// getLinkForAdapter returns the oslayer.Link corresponding to the given
// network adapter.
func (c *gcsCore) getLinkForAdapter(adapter prot.NetworkAdapter) (oslayer.Link, error) {
	id := adapter.AdapterInstanceId
	interfaceName, err := c.instanceIDToName(id)
	if err != nil {
		return nil, err
	}
	link, err := c.OS.GetLinkByName(interfaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get network interface for adapter %s", id)
	}
	return link, nil
}

// configureNetworkAdapter configures the given network adapter to be used by
// the container.
func (c *gcsCore) configureNetworkAdapter(adapter prot.NetworkAdapter) error {
	link, err := c.getLinkForAdapter(adapter)
	if err != nil {
		return err
	}
	if err := link.SetUp(); err != nil {
		return errors.Wrapf(err, "failed to set link up for adapter %s", adapter.AdapterInstanceId)
	}

	// Create the directory that will contain the resolv.conf file.
	if err := c.OS.MkdirAll(filepath.Join(baseFilesPath, "etc"), 0700); err != nil {
		return errors.Wrapf(err, "failed to create resolv.conf directory for adapter %s", adapter.AdapterInstanceId)
	}

	if adapter.NatEnabled {
		if err := c.configureNAT(adapter); err != nil {
			return errors.Wrapf(err, "failed to configure NAT on adapter %s", adapter.AdapterInstanceId)
		}
	} else {
		if err := c.configureWithDHCP(adapter); err != nil {
			return errors.Wrapf(err, "failed to configure DHCP on adapter %s", adapter.AdapterInstanceId)
		}
	}
	return nil
}

// configureNAT configures the given network adapter using the information
// specified in the NetworkAdapter struct.
func (c *gcsCore) configureNAT(adapter prot.NetworkAdapter) error {
	link, err := c.getLinkForAdapter(adapter)
	if err != nil {
		return err
	}

	// Set the route metric.
	metric := 1
	if adapter.EnableLowMetric {
		metric = 500
	}

	// Set the IP address.
	addr, err := c.OS.ParseAddr(fmt.Sprintf("%s/%d", adapter.AllocatedIpAddress, adapter.HostIpPrefixLength))
	if err != nil {
		return errors.Wrapf(err, "failed to parse allocated address for adapter %s", adapter.AdapterInstanceId)
	}
	if err := link.AddAddr(addr); err != nil {
		return errors.Wrapf(err, "failed to add address %s to adapter %s", addr.String(), adapter.AdapterInstanceId)
	}

	// Set the gateway route.
	if adapter.HostIpAddress != "" {
		addrString := adapter.HostIpAddress
		// If this isn't a valid CIDR address.
		if !strings.Contains(addrString, "/") {
			addrString += "/32"
		}
		addr, err := c.OS.ParseAddr(addrString)
		if err != nil {
			return errors.Wrapf(err, "invalid host IP address %s for adapter %s", adapter.HostIpAddress, adapter.AdapterInstanceId)
		}
		if err := c.OS.AddGatewayRoute(addr, link, metric); err != nil {
			return errors.Wrapf(err, "failed to set NAT route %s for adapter %s", adapter.HostIpAddress, adapter.AdapterInstanceId)
		}
	}

	// Set the DNS configuration.
	if err := c.generateResolvConfFile(adapter); err != nil {
		return errors.Wrapf(err, "failed to generate resolv.conf file for adapter %s", adapter.AdapterInstanceId)
	}

	return nil
}

// configureWithDHCP configures the given network adapter using DHCP.
func (c *gcsCore) configureWithDHCP(adapter prot.NetworkAdapter) error {
	// TODO: change this to dhclient -r <interfacename>
	id := adapter.AdapterInstanceId
	interfaceName, err := c.instanceIDToName(id)
	if err != nil {
		return err
	}
	out, err := c.OS.Command("udhcpc", "-q", "-i", interfaceName, "-s", "/sbin/udhcpc_config.script").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed call to udhcpc for adapter %s: %s", adapter.AdapterInstanceId, out)
	}
	resolvPath := filepath.Join(baseFilesPath, "etc/resolv.conf")
	exists, err := c.OS.PathExists(resolvPath)
	if err != nil {
		return errors.Wrapf(err, "failed to check if resolv.conf path already exists for adapter %s", adapter.AdapterInstanceId)
	}
	if !exists {
		if err := c.OS.Link("/etc/resolv.conf", resolvPath); err != nil {
			return errors.Wrapf(err, "failed to link resolv.conf file for adapter %s", adapter.AdapterInstanceId)
		}
	}
	return nil
}

// generateResolvConfFile generate a resolve.conf file in $baseFilesPath/etc
// for the given adapter.
// TODO: This method of managing DNS will potentially be replaced with another
// method in the future.
func (c *gcsCore) generateResolvConfFile(adapter prot.NetworkAdapter) error {
	fileContents := ""
	nameservers := strings.Split(adapter.HostDnsServerList, " ")
	for i, server := range nameservers {
		// Limit number of nameservers to 3.
		if i >= 3 {
			break
		}
		fileContents += fmt.Sprintf("nameserver %s\n", server)
	}
	fileContents += fmt.Sprintf("search %s\n", adapter.HostDnsSuffix)

	file, err := c.OS.OpenFile(filepath.Join(baseFilesPath, "etc", "resolv.conf"), os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		return errors.Wrapf(err, "failed to create resolv.conf file for adapter %s", adapter.AdapterInstanceId)
	}
	defer file.Close()
	if _, err := io.WriteString(file, fileContents); err != nil {
		return errors.Wrapf(err, "failed to write to resolv.conf file for adapter %s", adapter.AdapterInstanceId)
	}
	return nil
}

// moveAdapterIntoNamespace moves the given network adapter into the namespace
// of the container with the given ID.
func (c *gcsCore) moveAdapterIntoNamespace(id string, adapter prot.NetworkAdapter) error {
	// Get the root namespace, which should be the GCS's current namespace.
	rootNamespace, err := c.OS.GetCurrentNamespace()
	if err != nil {
		return errors.Wrap(err, "failed to get the root namespace")
	}
	defer rootNamespace.Close()

	// Get namespace information for the container process.
	containerPid, err := c.Rtime.GetInitPid(id)
	if err != nil {
		return errors.Wrapf(err, "failed to get the init process pid for container %s", id)
	}
	containerNamespace, err := c.OS.GetNamespaceFromPid(containerPid)
	if err != nil {
		return errors.Wrapf(err, "failed to get the namespace of process %d", containerPid)
	}
	defer containerNamespace.Close()

	// Move the interface into the container namespace.
	link, err := c.getLinkForAdapter(adapter)
	if err != nil {
		return errors.Wrapf(err, "failed to get the link for adapter %s in preparation for moving it into the container namespace", adapter.AdapterInstanceId)
	}
	addrs, err := link.Addrs(syscall.AF_INET)
	if err != nil {
		return errors.Wrapf(err, "failed to get the addrs for adapter %s", adapter.AdapterInstanceId)
	}
	gateways, err := link.GatewayRoutes(syscall.AF_INET)
	if err != nil {
		return errors.Wrapf(err, "failed to get the gateway routes for adapter %s", adapter.AdapterInstanceId)
	}
	if err := link.SetDown(); err != nil {
		return errors.Wrapf(err, "failed to set link down for adapter %s", adapter.AdapterInstanceId)
	}
	if err := link.SetNamespace(containerNamespace); err != nil {
		return errors.Wrapf(err, "failed to set the link to new namespace for adapter %s", adapter.AdapterInstanceId)
	}
	if err := c.OS.SetCurrentNamespace(containerNamespace); err != nil {
		return errors.Wrapf(err, "failed to set the namespace to the container namespace for container %s", id)
	}

	// Configure the interface with its original configuration.
	for _, addr := range addrs {
		// TODO: addr should never be nil
		if addr != nil {
			if err := link.AddAddr(addr); err != nil {
				return errors.Wrapf(err, "failed to set the IP address of the network interface for adapter %s", adapter.AdapterInstanceId)
			}
		}
	}
	if err := link.SetUp(); err != nil {
		return errors.Wrapf(err, "failed to set link up for adapter %s", adapter.AdapterInstanceId)
	}
	for _, route := range gateways {
		// TODO: gatewayRoute should never be nil
		if route != nil {
			link, err := c.OS.GetLinkByIndex(route.LinkIndex())
			if err != nil {
				return errors.Wrapf(err, "failed to get the link with index %d for adapter %s", route.LinkIndex(), adapter.AdapterInstanceId)
			}
			if err := c.OS.AddGatewayRoute(route.Gw(), link, route.Metric()); err != nil {
				return errors.Wrapf(err, "failed to add the gateway route for adapter %s", adapter.AdapterInstanceId)
			}
		}
	}

	// Change back to the root namespace.
	if err := c.OS.SetCurrentNamespace(rootNamespace); err != nil {
		return errors.Wrap(err, "failed to set the namespace to the root namespace")
	}
	return nil
}
