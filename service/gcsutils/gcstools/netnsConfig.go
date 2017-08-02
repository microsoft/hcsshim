package main

// This utility moves a network interface into a network namespace and
// configures it. The configuration is passed in as a JSON object
// (marshalled prot.NetworkAdapter).  It is necessary to implement
// this as a separate utility as in Go one does not have tight control
// over which OS thread a given Go thread/routing executes but as can
// only enter a namespace with a specific OS thread.
//
// Note, this logs to stdout so that the caller (gcs) can log the
// output itself.

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func netnsConfigMain() {
	if err := netnsConfig(); err != nil {
		log.Fatal("netnsConfig returned: ", err)
	}
	os.Exit(0)
}

func netnsConfig() error {
	ifStr := flag.String("if", "", "Interface/Adapter to move/configure")
	nspid := flag.Int("nspid", -1, "Process ID (to locate netns")
	cfgStr := flag.String("cfg", "", "Adapter configuration (json)")

	flag.Parse()
	if *ifStr == "" || *nspid == -1 || *cfgStr == "" {
		return fmt.Errorf("All three arguments must be specified")
	}

	var a prot.NetworkAdapter
	if err := json.Unmarshal([]byte(*cfgStr), &a); err != nil {
		return err
	}

	if a.NatEnabled {
		log.Infof("Configure %s in %d with: %s/%d gw=%s", *ifStr, *nspid, a.AllocatedIPAddress, a.HostIPPrefixLength, a.HostIPAddress)
	} else {
		log.Infof("Configure %s in %s with DHCP", *ifStr, *nspid)
	}

	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Stash current network namespace away and make sure we enter it as we leave
	origNS, err := netns.Get()
	if err != nil {
		return fmt.Errorf("netns.Get() failed: %v", err)
	}
	defer origNS.Close()

	// Get a reference to the new network namespace
	ns, err := netns.GetFromPid(*nspid)
	if err != nil {
		return fmt.Errorf("netns.GetFromPid(%d) failed: %v", *nspid, err)
	}
	defer ns.Close()

	// Get a reference to the interface and make sure it's down
	link, err := netlink.LinkByName(*ifStr)
	if err != nil {
		return fmt.Errorf("netlink.LinkByName(%s) failed: %v", *ifStr, err)
	}
	if err := netlink.LinkSetDown(link); err != nil {
		return fmt.Errorf("netlink.LinkSetDown(%#v) failed: %v", link, err)
	}

	// Move the interface to the new network namespace
	if err := netlink.LinkSetNsPid(link, *nspid); err != nil {
		return fmt.Errorf("netlink.SetNsPid(%#v, %d) failed: %v", link, *nspid, err)
	}

	log.Infof("Switching from %v to %v", origNS, ns)

	// Enter the new network namespace
	if err := netns.Set(ns); err != nil {
		return fmt.Errorf("netns.Set() failed: %v", err)
	}

	// Re-Get a reference to the interface (it may be a different ID in the new namespace)
	link, err = netlink.LinkByName(*ifStr)
	if err != nil {
		return fmt.Errorf("netlink.LinkByName(%s) failed: %v", *ifStr, err)
	}

	// Configure the interface
	if a.NatEnabled {
		metric := 1
		if a.EnableLowMetric {
			metric = 500
		}

		// Bring the interface up
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("netlink.LinkSetUp(%#v) failed: %v", link, err)
		}
		// Set IP address
		addr := &net.IPNet{
			IP: net.ParseIP(a.AllocatedIPAddress),
			// TODO(rn): This assumes/hardcodes IPv4
			Mask: net.CIDRMask(int(a.HostIPPrefixLength), 32)}
		ipAddr := &netlink.Addr{IPNet: addr, Label: ""}
		if err := netlink.AddrAdd(link, ipAddr); err != nil {
			return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %v", link, ipAddr, err)
		}
		// Set gateway
		if a.HostIPAddress != "" {
			gw := net.ParseIP(a.HostIPAddress)
			route := netlink.Route{
				Scope:     netlink.SCOPE_UNIVERSE,
				LinkIndex: link.Attrs().Index,
				Gw:        gw,
				Priority:  metric, // This is what ip route add does
			}
			if err := netlink.RouteAdd(&route); err != nil {
				return fmt.Errorf("netlink.RouteAdd(%#v) failed: %v", route, err)
			}
		}
	} else {
		err := exec.Command(
			"udhcpc",
			"-q",
			"-i", *ifStr,
			"-s", "/sbin/udhcpc_config.script").Run()
		if err != nil {
			return fmt.Errorf("udhcpc failed: %v", err)
		}
	}

	// Add some debug logging
	curNS, _ := netns.Get()
	// Refresh link attributes/state
	link, _ = netlink.LinkByIndex(link.Attrs().Index)
	attr := link.Attrs()
	addrs, _ := netlink.AddrList(link, 0)
	log.Infof("%v: %s[idx=%d,type=%s] is %v", curNS, attr.Name, attr.Index, link.Type(), attr.OperState)
	for _, addr := range addrs {
		log.Infof("  %v", addr)
	}

	return nil
}
