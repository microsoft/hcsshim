//go:build linux
// +build linux

package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/log"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// MoveInterfaceToNS moves the adapter with interface name `ifStr` to the network namespace
// of `pid`.
func MoveInterfaceToNS(ifStr string, pid int) error {
	// Get a reference to the interface and make sure it's down
	link, err := netlink.LinkByName(ifStr)
	if err != nil {
		return fmt.Errorf("netlink.LinkByName(%s) failed: %w", ifStr, err)
	}
	if err := netlink.LinkSetDown(link); err != nil {
		return fmt.Errorf("netlink.LinkSetDown(%#v) failed: %w", link, err)
	}

	// Move the interface to the new network namespace
	if err := netlink.LinkSetNsPid(link, pid); err != nil {
		return fmt.Errorf("netlink.SetNsPid(%#v, %d) failed: %w", link, pid, err)
	}
	return nil
}

// DoInNetNS is a utility to run a function `run` inside of a specific network namespace
// `ns`. This is accomplished by locking the current goroutines thread to prevent the goroutine
// from being scheduled to a new thread during execution of `run`. The threads original network namespace
// will be rejoined on exit.
func DoInNetNS(ns netns.NsHandle, run func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get current network namespace: %w", err)
	}
	defer origNs.Close()

	if err := netns.Set(ns); err != nil {
		return fmt.Errorf("failed to set network namespace to %v: %w", ns, err)
	}
	// Defer so we can re-enter the threads original netns on exit.
	defer netns.Set(origNs) //nolint:errcheck

	return run()
}

// NetNSConfig moves a network interface into a network namespace and
// configures it.
//
// This function MUST be used in tandem with `DoInNetNS` or some other means that ensures that the goroutine
// executing this code stays on the same thread.
func NetNSConfig(ctx context.Context, ifStr string, nsPid int, adapter *prot.NetworkAdapter) error {
	ctx, entry := log.S(ctx, logrus.Fields{
		"ifname": ifStr,
		"pid":    nsPid,
	})
	if ifStr == "" || nsPid == -1 || adapter == nil {
		return errors.New("All three arguments must be specified")
	}

	entry.Trace("Obtaining current namespace")
	ns, err := netns.Get()
	if err != nil {
		return fmt.Errorf("netns.Get() failed: %w", err)
	}
	defer ns.Close()
	entry.WithField("namespace", ns).Debug("New network namespace from PID")

	// Re-Get a reference to the interface (it may be a different ID in the new namespace)
	entry.Trace("Getting reference to interface")
	link, err := netlink.LinkByName(ifStr)
	if err != nil {
		return fmt.Errorf("netlink.LinkByName(%s) failed: %w", ifStr, err)
	}

	// User requested non-default MTU size
	if adapter.EncapOverhead != 0 {
		mtu := link.Attrs().MTU - int(adapter.EncapOverhead)
		entry.WithField("mtu", mtu).Debug("EncapOverhead non-zero, will set MTU")
		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return fmt.Errorf("netlink.LinkSetMTU(%#v, %d) failed: %w", link, mtu, err)
		}
	}

	// Configure the interface
	if adapter.NatEnabled {
		entry.Tracef("Configuring interface with NAT: %s/%d gw=%s",
			adapter.AllocatedIPAddress,
			adapter.HostIPPrefixLength, adapter.HostIPAddress)
		metric := 1
		if adapter.EnableLowMetric {
			metric = 500
		}

		// Bring the interface up
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("netlink.LinkSetUp(%#v) failed: %w", link, err)
		}
		if err := assignIPToLink(ctx, ifStr, nsPid, link,
			adapter.AllocatedIPAddress, adapter.HostIPAddress, adapter.HostIPPrefixLength,
			adapter.EnableLowMetric, metric,
		); err != nil {
			return err
		}
		if err := assignIPToLink(ctx, ifStr, nsPid, link,
			adapter.AllocatedIPv6Address, adapter.HostIPv6Address, adapter.HostIPv6PrefixLength,
			adapter.EnableLowMetric, metric,
		); err != nil {
			return err
		}
	} else {
		timeout := 30 * time.Second
		entry.Trace("Configure with DHCP")
		entry.WithField("timeout", timeout.String()).Debug("Execing udhcpc with timeout...")
		cmd := exec.Command("udhcpc", "-q", "-i", ifStr, "-s", "/sbin/udhcpc_config.script")

		done := make(chan error)
		go func() {
			done <- cmd.Wait()
		}()
		defer close(done)

		select {
		case <-time.After(timeout):
			var cos string
			co, err := cmd.CombinedOutput() // In case it has written something
			if err != nil {
				cos = string(co)
			}
			_ = cmd.Process.Kill()
			entry.WithField("timeout", timeout.String()).Warningf("udhcpc timed out [%s]", cos)
			return fmt.Errorf("udhcpc timed out. Failed to get DHCP address: %s", cos)
		case <-done:
			var cos string
			co, err := cmd.CombinedOutput() // Something should be on stderr
			if err != nil {
				cos = string(co)
			}
			if err != nil {
				entry.WithError(err).Debugf("udhcpc failed [%s]", cos)
				return fmt.Errorf("process failed (%s): %w", cos, err)
			}
		}
		var cos string
		co, err := cmd.CombinedOutput()
		if err != nil {
			cos = string(co)
		}
		entry.Debugf("udhcpc succeeded: %s", cos)
	}

	// Add some debug logging
	if entry.Logger.GetLevel() >= logrus.DebugLevel {
		curNS, _ := netns.Get()
		// Refresh link attributes/state
		link, _ = netlink.LinkByIndex(link.Attrs().Index)
		attr := link.Attrs()
		addrs, _ := netlink.AddrList(link, 0)
		addrsStr := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addrsStr = append(addrsStr, fmt.Sprintf("%v", addr))
		}

		entry.WithField("addresses", addrsStr).Debugf("%v: %s[idx=%d,type=%s] is %v",
			curNS, attr.Name, attr.Index, link.Type(), attr.OperState)
	}

	return nil
}

func assignIPToLink(ctx context.Context,
	ifStr string,
	nsPid int,
	link netlink.Link,
	allocatedIP string,
	gatewayIP string,
	prefixLen uint8,
	enableLowMetric bool,
	metric int,
) error {
	entry := log.G(ctx)
	entry.WithFields(logrus.Fields{
		"link":      link.Attrs().Name,
		"IP":        allocatedIP,
		"prefixLen": prefixLen,
		"gateway":   gatewayIP,
		"metric":    metric,
	}).Trace("assigning IP address")
	if allocatedIP == "" {
		return nil
	}
	// Set IP address
	ip, addr, err := net.ParseCIDR(allocatedIP + "/" + strconv.FormatUint(uint64(prefixLen), 10))
	if err != nil {
		return fmt.Errorf("parsing address %s/%d failed: %w", allocatedIP, prefixLen, err)
	}
	// the IP address field in addr is masked, so replace it with the original ip address
	addr.IP = ip
	entry.WithFields(logrus.Fields{
		"allocatedIP": ip,
		"IP":          addr,
	}).Debugf("parsed ip address %s/%d", allocatedIP, prefixLen)
	ipAddr := &netlink.Addr{IPNet: addr, Label: ""}
	if err := netlink.AddrAdd(link, ipAddr); err != nil {
		return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %w", link, ipAddr, err)
	}
	if gatewayIP == "" {
		return nil
	}
	// Set gateway
	gw := net.ParseIP(gatewayIP)
	if gw == nil {
		return fmt.Errorf("parsing gateway address %s failed: %w", gatewayIP, err)
	}

	if !addr.Contains(gw) {
		// In the case that a gw is not part of the subnet we are setting gw for,
		// a new addr containing this gw address need to be added into the link to avoid getting
		// unreachable error when adding this out-of-subnet gw route
		entry.Debugf("gw is outside of the subnet: Configure %s in %d with: %s/%d gw=%s\n",
			ifStr, nsPid, allocatedIP, prefixLen, gatewayIP)

		// net library's ParseIP call always returns an array of length 16, so we
		// need to first check if the address is IPv4 or IPv6 before calculating
		// the mask length. See https://pkg.go.dev/net#ParseIP.
		ml := 8
		if gw.To4() != nil {
			ml *= net.IPv4len
		} else if gw.To16() != nil {
			ml *= net.IPv6len
		} else {
			return fmt.Errorf("gw IP is neither IPv4 nor IPv6: %v", gw)
		}
		addr2 := &net.IPNet{
			IP:   gw,
			Mask: net.CIDRMask(ml, ml)}
		ipAddr2 := &netlink.Addr{IPNet: addr2, Label: ""}
		if err := netlink.AddrAdd(link, ipAddr2); err != nil {
			return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %w", link, ipAddr2, err)
		}
	}

	var table int
	if enableLowMetric {
		// add a route rule for the new interface so packets coming on this interface
		// always go out the same interface
		_, ml := addr.Mask.Size()
		srcNet := &net.IPNet{
			IP:   net.ParseIP(allocatedIP),
			Mask: net.CIDRMask(ml, ml),
		}
		rule := netlink.NewRule()
		rule.Table = 101
		rule.Src = srcNet
		rule.Priority = 5

		if err := netlink.RuleAdd(rule); err != nil {
			return fmt.Errorf("netlink.RuleAdd(%#v) failed: %w", rule, err)
		}
		table = rule.Table
	}
	// add the default route in that interface specific table
	route := netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: link.Attrs().Index,
		Gw:        gw,
		Table:     table,
		Priority:  metric,
	}
	if err := netlink.RouteAdd(&route); err != nil {
		return fmt.Errorf("netlink.RouteAdd(%#v) failed: %w", route, err)
	}
	return nil
}
