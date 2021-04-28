// +build linux

package network

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"time"

	"github.com/Microsoft/hcsshim/internal/guest/prot"
	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// MoveInterfaceToNS moves the adapter with interface name `ifStr` to the network namespace
// of `pid`.
func MoveInterfaceToNS(ifStr string, pid int) error {
	// Get a reference to the interface and make sure it's down
	link, err := netlink.LinkByName(ifStr)
	if err != nil {
		return errors.Wrapf(err, "netlink.LinkByName(%s) failed", ifStr)
	}
	if err := netlink.LinkSetDown(link); err != nil {
		return errors.Wrapf(err, "netlink.LinkSetDown(%#v) failed", link)
	}

	// Move the interface to the new network namespace
	if err := netlink.LinkSetNsPid(link, pid); err != nil {
		return errors.Wrapf(err, "netlink.SetNsPid(%#v, %d) failed", link, pid)
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
		return errors.Wrap(err, "failed to get current network namespace")
	}
	defer origNs.Close()

	if err := netns.Set(ns); err != nil {
		return errors.Wrapf(err, "failed to set network namespace to %v", ns)
	}
	// Defer so we can re-enter the threads original netns on exit.
	defer netns.Set(origNs)

	return run()
}

// NetNSConfig moves a network interface into a network namespace and
// configures it.
//
// This function MUST be used in tandem with `DoInNetNS` or some other means that ensures that the goroutine
// executing this code stays on the same thread.
func NetNSConfig(ctx context.Context, ifStr string, nsPid int, adapter *prot.NetworkAdapter) error {
	if ifStr == "" || nsPid == -1 || adapter == nil {
		return errors.New("All three arguments must be specified")
	}

	if adapter.NatEnabled {
		log.G(ctx).Debugf("Configure %s in %d with: %s/%d gw=%s", ifStr, nsPid, adapter.AllocatedIPAddress, adapter.HostIPPrefixLength, adapter.HostIPAddress)
	} else {
		log.G(ctx).Debugf("Configure %s in %d with DHCP", ifStr, nsPid)
	}

	log.G(ctx).Debug("Obtaining current namespace")
	ns, err := netns.Get()
	if err != nil {
		return errors.Wrap(err, "netns.Get() failed")
	}
	defer ns.Close()

	log.G(ctx).Debugf("New network namespace from PID %d is %v", nsPid, ns)

	// Re-Get a reference to the interface (it may be a different ID in the new namespace)
	log.G(ctx).Debug("Getting reference to interface")
	link, err := netlink.LinkByName(ifStr)
	if err != nil {
		return errors.Wrapf(err, "netlink.LinkByName(%s) failed", ifStr)
	}

	// User requested non-default MTU size
	if adapter.EncapOverhead != 0 {
		log.G(ctx).Debug("EncapOverhead non-zero, will set MTU")
		mtu := link.Attrs().MTU - int(adapter.EncapOverhead)
		log.G(ctx).Debugf("mtu %d", mtu)
		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return errors.Wrapf(err, "netlink.LinkSetMTU(%#v, %d) failed", link, mtu)
		}
	}

	// Configure the interface
	if adapter.NatEnabled {
		log.G(ctx).Debug("Nat enabled - configuring interface")
		metric := 1
		if adapter.EnableLowMetric {
			metric = 500
		}

		// Bring the interface up
		if err := netlink.LinkSetUp(link); err != nil {
			return errors.Wrapf(err, "netlink.LinkSetUp(%#v) failed", link)
		}
		// Set IP address
		addr := &net.IPNet{
			IP: net.ParseIP(adapter.AllocatedIPAddress),
			// TODO(rn): This assumes/hardcodes IPv4
			Mask: net.CIDRMask(int(adapter.HostIPPrefixLength), 32)}
		ipAddr := &netlink.Addr{IPNet: addr, Label: ""}
		if err := netlink.AddrAdd(link, ipAddr); err != nil {
			return errors.Wrapf(err, "netlink.AddrAdd(%#v, %#v) failed", link, ipAddr)
		}
		// Set gateway
		if adapter.HostIPAddress != "" {
			gw := net.ParseIP(adapter.HostIPAddress)

			if !addr.Contains(gw) {
				// In the case that a gw is not part of the subnet we are setting gw for,
				// a new addr containing this gw address need to be added into the link to avoid getting
				// unreachable error when adding this out-of-subnet gw route
				log.G(ctx).Debugf("gw is outside of the subnet: Configure %s in %d with: %s/%d gw=%s\n",
					ifStr, nsPid, adapter.AllocatedIPAddress, adapter.HostIPPrefixLength, adapter.HostIPAddress)
				addr2 := &net.IPNet{
					IP:   net.ParseIP(adapter.HostIPAddress),
					Mask: net.CIDRMask(32, 32)} // This assumes/hardcodes IPv4
				ipAddr2 := &netlink.Addr{IPNet: addr2, Label: ""}
				if err := netlink.AddrAdd(link, ipAddr2); err != nil {
					return errors.Wrapf(err, "netlink.AddrAdd(%#v, %#v) failed", link, ipAddr2)
				}
			}

			if !adapter.EnableLowMetric {
				route := netlink.Route{
					Scope:     netlink.SCOPE_UNIVERSE,
					LinkIndex: link.Attrs().Index,
					Gw:        gw,
					Priority:  metric, // This is what ip route add does
				}
				if err := netlink.RouteAdd(&route); err != nil {
					return errors.Wrapf(err, "netlink.RouteAdd(%#v) failed", route)
				}
			} else {
				// add a route rule for the new interface so packets coming on this interface
				// always go out the same interface
				srcNet := &net.IPNet{IP: net.ParseIP(adapter.AllocatedIPAddress), Mask: net.CIDRMask(32, 32)}
				rule := netlink.NewRule()
				rule.Table = 101
				rule.Src = srcNet
				rule.Priority = 5

				if err := netlink.RuleAdd(rule); err != nil {
					return errors.Wrapf(err, "netlink.RuleAdd(%#v) failed", rule)
				}

				// add the default route in that interface specific table
				route := netlink.Route{
					Scope:     netlink.SCOPE_UNIVERSE,
					LinkIndex: link.Attrs().Index,
					Gw:        gw,
					Table:     rule.Table,
					Priority:  metric,
				}
				if err := netlink.RouteAdd(&route); err != nil {
					return errors.Wrapf(err, "netlink.RouteAdd(%#v) failed", route)
				}

			}
		}
	} else {
		log.G(ctx).Debug("Execing udhcpc with timeout...")
		cmd := exec.Command("udhcpc", "-q", "-i", ifStr, "-s", "/sbin/udhcpc_config.script")

		done := make(chan error)
		go func() {
			done <- cmd.Wait()
		}()
		defer close(done)

		select {
		case <-time.After(30 * time.Second):
			var cos string
			co, err := cmd.CombinedOutput() // In case it has written something
			if err != nil {
				cos = string(co)
			}
			cmd.Process.Kill()
			log.G(ctx).Debugf("udhcpc timed out [%s]", cos)
			return fmt.Errorf("udhcpc timed out. Failed to get DHCP address: %s", cos)
		case err := <-done:
			var cos string
			co, err := cmd.CombinedOutput() // Something should be on stderr
			if err != nil {
				cos = string(co)
			}
			if err != nil {
				log.G(ctx).WithError(err).Debugf("udhcpc failed [%s]", cos)
				return errors.Wrapf(err, "process failed (%s)", cos)
			}
		}
		var cos string
		co, err := cmd.CombinedOutput()
		if err != nil {
			cos = string(co)
		}
		log.G(ctx).Debugf("udhcpc succeeded: %s", cos)
	}

	// Add some debug logging
	curNS, _ := netns.Get()
	// Refresh link attributes/state
	link, _ = netlink.LinkByIndex(link.Attrs().Index)
	attr := link.Attrs()
	addrs, _ := netlink.AddrList(link, 0)

	log.G(ctx).Debugf("%v: %s[idx=%d,type=%s] is %v", curNS, attr.Name, attr.Index, link.Type(), attr.OperState)
	for _, addr := range addrs {
		log.G(ctx).Debugf("  %v", addr)
	}

	return nil
}
