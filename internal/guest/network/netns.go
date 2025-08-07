//go:build linux
// +build linux

package network

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Microsoft/hcsshim/internal/log"
	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

var (
	// function definitions for mocking configureLink
	netlinkAddrAdd  = netlink.AddrAdd
	netlinkRouteAdd = netlink.RouteAdd
	netlinkRuleAdd  = netlink.RuleAdd
)

const (
	ipv4GwDestination = "0.0.0.0/0"
	ipv4EmptyGw       = "0.0.0.0"
	ipv6GwDestination = "::/0"
	ipv6EmptyGw       = "::"

	unreachableErrStr = "network is unreachable"
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
	defer netns.Set(origNs) //nolint:errcheck

	return run()
}

// NetNSConfig moves a network interface into a network namespace and
// configures it.
//
// This function MUST be used in tandem with `DoInNetNS` or some other means that ensures that the goroutine
// executing this code stays on the same thread.
func NetNSConfig(ctx context.Context, ifStr string, nsPid int, adapter *guestresource.LCOWNetworkAdapter) error {
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
		return errors.Wrap(err, "netns.Get() failed")
	}
	defer ns.Close()
	entry.WithField("namespace", ns).Debug("New network namespace from PID")

	// Re-Get a reference to the interface (it may be a different ID in the new namespace)
	entry.Trace("Getting reference to interface")
	link, err := netlink.LinkByName(ifStr)
	if err != nil {
		return errors.Wrapf(err, "netlink.LinkByName(%s) failed", ifStr)
	}

	// User requested non-default MTU size
	if adapter.EncapOverhead != 0 {
		mtu := link.Attrs().MTU - int(adapter.EncapOverhead)
		entry.WithField("mtu", mtu).Debug("EncapOverhead non-zero, will set MTU")
		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return errors.Wrapf(err, "netlink.LinkSetMTU(%#v, %d) failed", link, mtu)
		}
	}

	// Configure the interface
	if len(adapter.IPConfigs) != 0 {
		entry.Debugf("Configuring interface with NAT: %v", adapter)

		// Bring the interface up
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("netlink.LinkSetUp(%#v) failed: %w", link, err)
		}
		if err := configureLink(ctx, link, adapter); err != nil {
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
				return errors.Wrapf(err, "process failed (%s)", cos)
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
	if entry.Logger.IsLevelEnabled(logrus.DebugLevel) {
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

func configureLink(ctx context.Context,
	link netlink.Link,
	adapter *guestresource.LCOWNetworkAdapter,
) error {
	var table int
	for _, ipConfig := range adapter.IPConfigs {
		log.G(ctx).WithFields(logrus.Fields{
			"link":      link.Attrs().Name,
			"IP":        ipConfig.IPAddress,
			"prefixLen": ipConfig.PrefixLength,
		}).Debug("assigning IP address")

		// Set IP address
		ip, addr, err := net.ParseCIDR(ipConfig.IPAddress + "/" + strconv.FormatUint(uint64(ipConfig.PrefixLength), 10))
		if err != nil {
			return fmt.Errorf("parsing address %s/%d failed: %w", ipConfig.IPAddress, ipConfig.PrefixLength, err)
		}
		// the IP address field in addr is masked, so replace it with the original ip address
		addr.IP = ip
		log.G(ctx).WithFields(logrus.Fields{
			"allocatedIP": ip,
			"IP":          addr,
		}).Debugf("parsed ip address %s/%d", ipConfig.IPAddress, ipConfig.PrefixLength)
		ipAddr := &netlink.Addr{IPNet: addr, Label: ""}
		if err := netlinkAddrAdd(link, ipAddr); err != nil {
			return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %w", link, ipAddr, err)
		}

		if adapter.EnableLowMetric {
			// add a route rule for the new interface so packets coming on this interface
			// always go out the same interface
			_, ml := addr.Mask.Size()
			srcNet := &net.IPNet{
				IP:   net.ParseIP(ipConfig.IPAddress),
				Mask: net.CIDRMask(ml, ml),
			}
			rule := netlink.NewRule()
			rule.Table = 101
			rule.Src = srcNet
			rule.Priority = 5

			if err := netlinkRuleAdd(rule); err != nil {
				return errors.Wrapf(err, "netlink.RuleAdd(%#v) failed", rule)
			}
			table = rule.Table
		}
	}

	for _, r := range adapter.Routes {
		log.G(ctx).WithField("route", r).Debugf("adding a route to interface %s", link.Attrs().Name)

		if (r.DestinationPrefix == ipv4GwDestination || r.DestinationPrefix == ipv6GwDestination) &&
			(r.NextHop == ipv4EmptyGw || r.NextHop == ipv6EmptyGw) {
			// this indicates no default gateway was added for this interface
			continue
		}

		// dst will be nil when setting default gateways
		var dst *net.IPNet
		if r.DestinationPrefix != ipv4GwDestination && r.DestinationPrefix != ipv6GwDestination {
			dstIP, dstAddr, err := net.ParseCIDR(r.DestinationPrefix)
			if err != nil {
				return fmt.Errorf("parsing route dst address %s failed: %w", r.DestinationPrefix, err)
			}
			dstAddr.IP = dstIP
			dst = dstAddr
		}

		// gw can be nil when setting something like
		// ip route add 10.0.0.0/16 dev eth0
		gw := net.ParseIP(r.NextHop)
		if gw == nil && dst == nil {
			return fmt.Errorf("gw and destination cannot both be nil")
		}

		metric := int(r.Metric)
		if adapter.EnableLowMetric && r.Metric == 0 {
			// set a low metric only if the endpoint didn't already have a metric
			// configured
			metric = 500
		}
		route := netlink.Route{
			Scope:     netlink.SCOPE_UNIVERSE,
			LinkIndex: link.Attrs().Index,
			Gw:        gw,
			Dst:       dst,
			Priority:  metric,
			// table will be set to 101 for the legacy policy based routing support
			Table: table,
		}
		if err := netlinkRouteAdd(&route); err != nil {
			// unfortunately, netlink library doesn't have great error handling,
			// so we have to rely on the string error here
			if strings.Contains(err.Error(), unreachableErrStr) && gw != nil {
				// In the case that a gw is not part of the subnet we are setting gw for,
				// a new addr containing this gw address needs to be added into the link to avoid getting
				// unreachable error when adding this out-of-subnet gw route
				log.G(ctx).Infof("gw is outside of the subnet: %v", gw)

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
				if err := netlinkAddrAdd(link, ipAddr2); err != nil {
					return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %w", link, ipAddr2, err)
				}

				// try adding the route again
				if err := netlinkRouteAdd(&route); err != nil {
					return fmt.Errorf("netlink.RouteAdd(%#v) failed: %w", route, err)
				}
			} else {
				return fmt.Errorf("netlink.RouteAdd(%#v) failed: %w", route, err)
			}
		}
	}
	return nil
}
