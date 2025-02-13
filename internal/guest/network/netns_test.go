//go:build linux
// +build linux

package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
	"github.com/vishvananda/netlink"
)

const (
	ipv4TotalMaskLength = 32
	ipv6TotalMaskLength = 128
)

type testRoute struct {
	scope    netlink.Scope
	dstIP    string
	gw       string
	priority int
}

type testAddr struct {
	ip         string
	prefixLen  int
	maskLength int
}

type testRule struct {
	table      int
	priority   int
	ip         string
	maskLength int
}

type fakeLink struct {
	attr *netlink.LinkAttrs
}

func (l *fakeLink) Attrs() *netlink.LinkAttrs {
	return l.attr
}

func (l *fakeLink) Type() string {
	return ""
}

func newFakeLink(name string, index int) *fakeLink {
	attr := &netlink.LinkAttrs{
		Name:  name,
		Index: index,
	}
	return &fakeLink{
		attr: attr,
	}
}

var _ = (netlink.Link)(&fakeLink{})

// unreachableNetlinkRouteAdd is a helper function that will always return that the
// network is unreachable
func unreachableNetlinkRouteAdd(count *int, link netlink.Link, expected []*testRoute) func(_ *netlink.Route) error {
	return func(route *netlink.Route) error {
		f := standardNetlinkRouteAdd(count, link, expected)
		if err := f(route); err != nil {
			return err
		}
		return errors.New(unreachableErrStr)
	}
}

func standardNetlinkRouteAdd(count *int, link netlink.Link, expected []*testRoute) func(_ *netlink.Route) error {
	return func(route *netlink.Route) error {
		if *count >= len(expected) {
			return fmt.Errorf("expected to call route add %d times, instead got %d", len(expected), *count)
		}
		exp := expected[*count]

		if exp.scope != route.Scope {
			return fmt.Errorf("expected scope %s, instead got %s", exp.scope, route.Scope)
		}

		if route.Gw == nil && exp.gw != "" {
			return fmt.Errorf("expected to have gw set %s", exp.gw)
		} else if route.Gw != nil && (exp.gw != route.Gw.String()) {
			return fmt.Errorf("expected gw %s, instead got %s", exp.gw, route.Gw.String())
		}

		if route.Dst == nil && exp.dstIP != "" {
			return fmt.Errorf("expected to have dst set %s", exp.dstIP)
		} else if route.Dst != nil && (exp.dstIP != route.Dst.String()) {
			return fmt.Errorf("expected dst %s, instead got %s", exp.dstIP, route.Dst.String())
		}

		if route.Priority != exp.priority {
			return fmt.Errorf("expected to use metric %d, instead used %d", exp.priority, route.Priority)
		}

		if link.Attrs().Index != route.LinkIndex {
			return fmt.Errorf("expected to get link index %d, instead got %d", link.Attrs().Index, route.LinkIndex)
		}

		*count++
		return nil
	}
}

func standardNetlinkAddrAdd(count *int, expected []*testAddr) func(_ netlink.Link, _ *netlink.Addr) error {
	return func(link netlink.Link, addr *netlink.Addr) error {
		if *count >= len(expected) {
			return fmt.Errorf("expected to call addr add %d times, instead got %d", len(expected), *count)
		}
		exp := expected[*count]

		if addr.IP.String() != exp.ip {
			return fmt.Errorf("expected to add address %s, instead got %s", exp.ip, addr.IP.String())
		}
		expectedMask := net.CIDRMask(exp.prefixLen, exp.maskLength)
		if !bytes.Equal(addr.Mask, expectedMask) {
			return fmt.Errorf("expected mask to be %s, instead got %s", expectedMask, addr.Mask)
		}
		*count++
		return nil
	}
}

func standardNetlinkRuleAdd(count *int, expected []*testRule) func(rule *netlink.Rule) error {
	return func(rule *netlink.Rule) error {
		if *count >= len(expected) {
			return fmt.Errorf("expected to call addr add %d times, instead got %d", len(expected), *count)
		}
		exp := expected[*count]

		if exp.table != rule.Table {
			return fmt.Errorf("expected table to be %d, instead got %d", exp.table, rule.Table)
		}
		if exp.priority != rule.Priority {
			return fmt.Errorf("expected priority to be %d, instead got %d", exp.priority, rule.Priority)
		}
		if exp.ip != rule.Src.IP.String() {
			return fmt.Errorf("expected src IP to be %s, instead got %s", exp.ip, rule.Src.IP.String())
		}
		expectedMask := net.CIDRMask(exp.maskLength, exp.maskLength)
		if !bytes.Equal(expectedMask, rule.Src.Mask) {
			return fmt.Errorf("expected mask to be %s, instead got %s", expectedMask, rule.Src.Mask)
		}
		*count++
		return nil
	}
}

func Test_configureLink_IPv4(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "192.168.0.100",
				DestinationPrefix: ipv4GwDestination,
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "192.168.0.100",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "192.168.0.5",
			prefixLen:  24,
			maskLength: ipv4TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_EnableLowMetric_IPv4(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "192.168.0.100",
				DestinationPrefix: ipv4GwDestination,
			},
		},
		EnableLowMetric: true,
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "192.168.0.100",
			priority: 500, // enable low metric sets the metric to 500
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "192.168.0.5",
			prefixLen:  24,
			maskLength: ipv4TotalMaskLength,
		},
	}
	expectedRule := []*testRule{
		{
			table:      101,
			priority:   5,
			ip:         "192.168.0.5",
			maskLength: ipv4TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	ruleAddCount := 0
	netlinkRuleAdd = standardNetlinkRuleAdd(&ruleAddCount, expectedRule)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}

	if ruleAddCount != len(expectedRule) {
		t.Fatalf("expected to call ruleAdd %d times, instead called it %d times", len(expectedRule), ruleAddCount)
	}
}

func Test_configureLink_IPv6(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
				PrefixLength: 64,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
				DestinationPrefix: ipv6GwDestination,
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			prefixLen:  64,
			maskLength: ipv6TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_EnableLowMetric_IPv6(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
				PrefixLength: 64,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
				DestinationPrefix: ipv6GwDestination,
			},
		},
		EnableLowMetric: true,
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			priority: 500, // enable low metric sets the metric to 500
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			prefixLen:  64,
			maskLength: ipv6TotalMaskLength,
		},
	}
	expectedRule := []*testRule{
		{
			table:      101,
			priority:   5,
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			maskLength: ipv6TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	ruleAddCount := 0
	netlinkRuleAdd = standardNetlinkRuleAdd(&ruleAddCount, expectedRule)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}

	if ruleAddCount != len(expectedRule) {
		t.Fatalf("expected to call ruleAdd %d times, instead called it %d times", len(expectedRule), ruleAddCount)
	}
}

func Test_configureLink_IPv4AndIPv6(t *testing.T) {
	ctx := context.Background()

	link1 := newFakeLink("eth0", 1)

	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
			{
				IPAddress:    "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
				PrefixLength: 64,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "192.168.0.100",
				DestinationPrefix: ipv4GwDestination,
			},
			{
				NextHop:           "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
				DestinationPrefix: ipv6GwDestination,
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "192.168.0.100",
			priority: 0,
		},
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "192.168.0.5",
			prefixLen:  24,
			maskLength: ipv4TotalMaskLength,
		},
		{
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			prefixLen:  64,
			maskLength: ipv6TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_No_Gateway_IPv4(t *testing.T) {
	ctx := context.Background()

	link1 := newFakeLink("eth0", 0)

	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "192.168.0.5",
			prefixLen:  24,
			maskLength: ipv4TotalMaskLength,
		},
	}

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)
	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_No_Gateway_IPv6(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
				PrefixLength: 64,
			},
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			prefixLen:  64,
			maskLength: ipv6TotalMaskLength,
		},
	}

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)
	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_GatewayOutsideSubnet_IPv4(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "10.0.0.2",
				DestinationPrefix: ipv4GwDestination,
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "10.0.0.2",
			priority: 0,
		},
		{
			// routeAdd should be called twice with the same content in this test
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "10.0.0.2",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "192.168.0.5",
			prefixLen:  24,
			maskLength: ipv4TotalMaskLength,
		},
		{
			ip:         "10.0.0.2",
			prefixLen:  ipv4TotalMaskLength,
			maskLength: ipv4TotalMaskLength,
		},
	}

	// since it isn't easy to change the definition of the netlinkRouteAdd per call,
	// instead of checking for a success for this test case, we just check that the
	// behavior we expect happens when we get the error message we expect.
	routeAddCount := 0
	netlinkRouteAdd = unreachableNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	err := configureLink(ctx, link1, adapter)
	if err == nil || !strings.Contains(err.Error(), unreachableErrStr) {
		t.Fatalf("expected an error from configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_GatewayOutsideSubnet_IPv6(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
				PrefixLength: 64,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           "9999:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
				DestinationPrefix: ipv6GwDestination,
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9999:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			priority: 0,
		},
		{
			// routeAdd should be called twice with the same content in this test
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9999:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			prefixLen:  64,
			maskLength: ipv6TotalMaskLength,
		},
		{
			ip:         "9999:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			prefixLen:  ipv6TotalMaskLength,
			maskLength: ipv6TotalMaskLength,
		},
	}

	// since it isn't easy to change the definition of the netlinkRouteAdd per call,
	// instead of checking for a success for this test case, we just check that the
	// behavior we expect happens when we get the error message we expect.
	routeAddCount := 0
	netlinkRouteAdd = unreachableNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	err := configureLink(ctx, link1, adapter)
	if err == nil || !strings.Contains(err.Error(), unreachableErrStr) {
		t.Fatalf("expected an error from configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_MultiRoute_IPv4(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           ipv4EmptyGw,
				DestinationPrefix: ipv4GwDestination,
			},
			{
				NextHop:           "192.168.0.100",
				DestinationPrefix: ipv4GwDestination,
			},
			{
				NextHop:           "192.168.0.5",
				DestinationPrefix: "10.10.0.0/16",
			},
			{
				DestinationPrefix: "10.0.0.0/16",
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "192.168.0.100",
			priority: 0,
		},
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "192.168.0.5",
			dstIP:    "10.10.0.0/16",
			priority: 0,
		},
		{
			scope:    netlink.SCOPE_UNIVERSE,
			dstIP:    "10.0.0.0/16",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "192.168.0.5",
			prefixLen:  24,
			maskLength: ipv4TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_MultiRoute_IPv6(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
				PrefixLength: 64,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				NextHop:           ipv6EmptyGw,
				DestinationPrefix: ipv6GwDestination,
			},
			{
				NextHop:           "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
				DestinationPrefix: ipv6GwDestination,
			},
			{
				NextHop:           "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:cccc",
				DestinationPrefix: "fc49:65e9:61e8:5aa7:9680:dd25:8ce8:c4f0/64",
			},
			{
				DestinationPrefix: "25a6:6c50:5564:4a67:d7d3:6aa3:7e1f:9786/64",
			},
		},
	}
	expectedRoutes := []*testRoute{
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
			priority: 0,
		},
		{
			scope:    netlink.SCOPE_UNIVERSE,
			gw:       "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:cccc",
			dstIP:    "fc49:65e9:61e8:5aa7:9680:dd25:8ce8:c4f0/64",
			priority: 0,
		},
		{
			scope:    netlink.SCOPE_UNIVERSE,
			dstIP:    "25a6:6c50:5564:4a67:d7d3:6aa3:7e1f:9786/64",
			priority: 0,
		},
	}
	expectedAddr := []*testAddr{
		{
			ip:         "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			prefixLen:  64,
			maskLength: ipv6TotalMaskLength,
		},
	}

	routeAddCount := 0
	netlinkRouteAdd = standardNetlinkRouteAdd(&routeAddCount, link1, expectedRoutes)

	addrAddCount := 0
	netlinkAddrAdd = standardNetlinkAddrAdd(&addrAddCount, expectedAddr)

	if err := configureLink(ctx, link1, adapter); err != nil {
		t.Fatalf("configureLink: %s", err)
	}

	if routeAddCount != len(expectedRoutes) {
		t.Fatalf("expected to call routeAdd %d times, instead called it %d times", len(expectedRoutes), routeAddCount)
	}

	if addrAddCount != len(expectedAddr) {
		t.Fatalf("expected to call addrAdd %d times, instead called it %d times", len(expectedAddr), addrAddCount)
	}
}

func Test_configureLink_Bad_Route_IPv4(t *testing.T) {
	ctx := context.Background()
	link1 := newFakeLink("eth0", 0)
	adapter := &guestresource.LCOWNetworkAdapter{
		IPConfigs: []guestresource.LCOWIPConfig{
			{
				IPAddress:    "192.168.0.5",
				PrefixLength: 24,
			},
		},
		Routes: []guestresource.LCOWRoute{
			{
				// this is a bad route, destination prefix cannot be empty.
				// Default gateways should have a destination prefix of "0.0.0.0/0"
				NextHop: "192.168.0.100",
			},
		},
	}

	err := configureLink(ctx, link1, adapter)
	if err == nil {
		t.Fatal("configureLink expected error due to badly formed route")
	}
}
