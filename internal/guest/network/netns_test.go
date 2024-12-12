//go:build linux
// +build linux

package network

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

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

func standardNetlinkAddrAdd(expectedIP string, prefixLen, totalMaskSize int) func(_ netlink.Link, _ *netlink.Addr) error {
	return func(link netlink.Link, addr *netlink.Addr) error {
		if addr.IP.String() != expectedIP {
			return fmt.Errorf("expected to add address %s, instead got %s", expectedIP, addr.IP.String())
		}
		expectedMask := net.CIDRMask(prefixLen, totalMaskSize)
		if !bytes.Equal(addr.Mask, expectedMask) {
			return fmt.Errorf("expected mask to be %s, instead got %s", expectedMask, addr.Mask)
		}
		return nil
	}
}

func standardNetlinkRouteAdd(gatewayIP string, table, metric int) func(_ *netlink.Route) error {
	return func(route *netlink.Route) error {
		if route.Gw.String() != gatewayIP {
			return fmt.Errorf("expected to add gateway %s, instead got %s", gatewayIP, route.Gw.String())
		}
		if route.Table != table {
			return fmt.Errorf("expected to use table %d, instead got %d", table, route.Table)
		}
		if route.Priority != metric {
			return fmt.Errorf("expected to use metric %d, instead used %d", metric, route.Priority)
		}
		return nil
	}
}

type assignIPToLinkTest struct {
	name          string
	ifStr         string
	allocatedIP   string
	gatewayIP     string
	prefixLen     uint8
	totalMaskSize int
}

var defaultAssignIPToLinkTests = []assignIPToLinkTest{
	{
		name:          "ipv4 standard",
		ifStr:         "eth0",
		allocatedIP:   "192.168.0.5",
		gatewayIP:     "192.168.0.100",
		prefixLen:     uint8(24),
		totalMaskSize: 32,
	},
	{
		name:          "ipv6 standard",
		ifStr:         "eth0",
		allocatedIP:   "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
		gatewayIP:     "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:aaaa",
		prefixLen:     uint8(64),
		totalMaskSize: 128,
	},
}

func Test_AssignIPToLink(t *testing.T) {
	ctx := context.Background()

	for _, tt := range defaultAssignIPToLinkTests {
		t.Run(tt.name, func(st *testing.T) {
			link1 := newFakeLink(tt.ifStr, 0)

			netlinkAddrAdd = standardNetlinkAddrAdd(tt.allocatedIP, int(tt.prefixLen), tt.totalMaskSize)
			netlinkRouteAdd = standardNetlinkRouteAdd(tt.gatewayIP, 0, 1)

			if err := assignIPToLink(ctx, tt.ifStr, 10, link1, tt.allocatedIP, tt.gatewayIP, tt.prefixLen, false, 1); err != nil {
				st.Fatalf("assignIPToLink: %s", err)
			}
		})
	}

}

func Test_AssignIPToLink_No_Gateway(t *testing.T) {
	ctx := context.Background()

	for _, tt := range defaultAssignIPToLinkTests {
		t.Run(tt.name, func(st *testing.T) {
			// remove the gateway IP set for the tests
			tt.gatewayIP = ""
			link1 := newFakeLink(tt.ifStr, 0)

			netlinkAddrAdd = standardNetlinkAddrAdd(tt.allocatedIP, int(tt.prefixLen), tt.totalMaskSize)
			netlinkRouteAdd = standardNetlinkRouteAdd(tt.gatewayIP, 0, 1)

			if err := assignIPToLink(ctx, tt.ifStr, 10, link1, tt.allocatedIP, tt.gatewayIP, tt.prefixLen, false, 1); err != nil {
				st.Fatalf("assignIPToLink: %s", err)
			}
		})
	}

}

func Test_AssignIPToLink_GatewayOutsideSubnet(t *testing.T) {
	ctx := context.Background()

	var assignIPToLinkTestsGateway = []assignIPToLinkTest{
		{
			name:          "ipv4 standard",
			ifStr:         "eth0",
			allocatedIP:   "192.168.0.5",
			gatewayIP:     "10.0.0.5",
			prefixLen:     uint8(24),
			totalMaskSize: 32,
		},
		{
			name:          "ipv6 standard",
			ifStr:         "eth0",
			allocatedIP:   "9541:a2d4:f0f3:18ff:c868:26ce:e9c4:30a6",
			gatewayIP:     "337c:83ab:b4cc:d823:6b5d:6aea:f605:80c5",
			prefixLen:     uint8(64),
			totalMaskSize: 128,
		},
	}

	for _, tt := range assignIPToLinkTestsGateway {
		t.Run(tt.name, func(st *testing.T) {
			link1 := newFakeLink(tt.ifStr, 0)

			netlinkAddCalls := 0
			netlinkAddrAdd = func(link netlink.Link, addr *netlink.Addr) error {
				expectedIP := tt.allocatedIP
				expectedMask := net.CIDRMask(int(tt.prefixLen), tt.totalMaskSize)
				if netlinkAddCalls != 0 {
					// on the second call, we want to check for the gateway address being added
					expectedIP = tt.gatewayIP
					expectedMask = net.CIDRMask(tt.totalMaskSize, tt.totalMaskSize)
				}
				if addr.IP.String() != expectedIP {
					return fmt.Errorf("expected to add address %s, instead got %s", expectedIP, addr.IP.String())
				}
				if !bytes.Equal(addr.Mask, expectedMask) {
					return fmt.Errorf("expected mask to be %s, instead got %s", expectedMask, addr.Mask)
				}
				netlinkAddCalls++
				return nil
			}

			netlinkRouteAdd = standardNetlinkRouteAdd(tt.gatewayIP, 0, 1)

			if err := assignIPToLink(ctx, tt.ifStr, 10, link1, tt.allocatedIP, tt.gatewayIP, tt.prefixLen, false, 1); err != nil {
				st.Fatalf("assignIPToLink: %s", err)
			}

			if netlinkAddCalls < 2 {
				st.Fatalf("expected to call netlink AddrAdd %d times, instead got %d times", 2, netlinkAddCalls)
			}
		})
	}

}

func Test_AssignIPToLink_EnableLowMetric(t *testing.T) {
	ctx := context.Background()
	table := 101
	metric := 500

	for _, tt := range defaultAssignIPToLinkTests {
		t.Run(tt.name, func(st *testing.T) {
			link1 := newFakeLink(tt.ifStr, 0)

			netlinkAddrAdd = standardNetlinkAddrAdd(tt.allocatedIP, int(tt.prefixLen), tt.totalMaskSize)
			netlinkRouteAdd = standardNetlinkRouteAdd(tt.gatewayIP, table, metric)

			netlinkRuleAddCalled := false
			netlinkRuleAdd = func(rule *netlink.Rule) error {
				netlinkRuleAddCalled = true
				if rule.Src.IP.String() != tt.allocatedIP {
					return fmt.Errorf("expected to add rule for address %s, instead got %s", tt.allocatedIP, rule.Src.IP.String())
				}
				expectedMask := net.CIDRMask(tt.totalMaskSize, tt.totalMaskSize)
				if !bytes.Equal(expectedMask, rule.Src.Mask) {
					return fmt.Errorf("expected mask to be %s, instead got %s", expectedMask, rule.Src.Mask)
				}
				return nil
			}

			if err := assignIPToLink(ctx, tt.ifStr, 10, link1, tt.allocatedIP, tt.gatewayIP, tt.prefixLen, true, metric); err != nil {
				t.Fatalf("assignIPToLink: %s", err)
			}

			if !netlinkRuleAddCalled {
				t.Fatal("should have added a rule since enableLowMetric was set")
			}
		})
	}

}
