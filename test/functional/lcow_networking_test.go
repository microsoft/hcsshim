//go:build windows && functional
// +build windows,functional

package functional

import (
	"context"
	"fmt"
	"strings"
	"testing"

	ctrdoci "github.com/containerd/containerd/v2/pkg/oci"

	"github.com/Microsoft/hcsshim/hcn"
	"github.com/Microsoft/hcsshim/osversion"

	testcmd "github.com/Microsoft/hcsshim/test/internal/cmd"
	testcontainer "github.com/Microsoft/hcsshim/test/internal/container"
	testlayers "github.com/Microsoft/hcsshim/test/internal/layers"
	testoci "github.com/Microsoft/hcsshim/test/internal/oci"
	"github.com/Microsoft/hcsshim/test/internal/util"
	"github.com/Microsoft/hcsshim/test/pkg/require"
	testuvm "github.com/Microsoft/hcsshim/test/pkg/uvm"
)

func TestLCOW_IPv6_Assignment(t *testing.T) {
	requireFeatures(t, featureLCOW, featureUVM)
	require.Build(t, osversion.RS5)

	ns, err := newNetworkNamespace()
	if err != nil {
		t.Fatalf("namespace creation: %v", err)
	}
	t.Cleanup(func() {
		if err := ns.Delete(); err != nil {
			t.Errorf("namespace delete: %v", err)
		}
	})
	t.Logf("created namespace %s", ns.Id)

	ipv4Route := hcn.Route{
		NextHop:           "192.168.128.1",
		DestinationPrefix: "0.0.0.0/0",
	}

	ipv6Route := hcn.Route{
		NextHop:           "fd00::101",
		DestinationPrefix: "::/0",
	}

	// create network and endpoint
	ntwk, err := (&hcn.HostComputeNetwork{
		Name: hcsOwner + "network",
		Type: hcn.NAT,
		Ipams: []hcn.Ipam{
			{
				Type: "Static",
				Subnets: []hcn.Subnet{
					{
						IpAddressPrefix: "192.168.128.0/20",
						Routes:          []hcn.Route{ipv4Route},
					},
					{
						IpAddressPrefix: "fd00::100/120",
						Routes:          []hcn.Route{ipv6Route},
					},
				},
			},
		},
		SchemaVersion: hcn.Version{Major: 2, Minor: 2},
	}).Create()
	if err != nil {
		t.Fatalf("network creation: %v", err)
	}
	t.Cleanup(func() {
		if err := ntwk.Delete(); err != nil {
			t.Errorf("network delete: %v", err)
		}
	})
	t.Logf("created network %s (%s)", ntwk.Name, ntwk.Id)

	ip4Want := hcn.IpConfig{
		IpAddress:    "192.168.128.4",
		PrefixLength: 20,
	}
	ip6Want := hcn.IpConfig{
		IpAddress:    "fd00::106",
		PrefixLength: 120,
	}

	ep, err := (&hcn.HostComputeEndpoint{
		Name:               ntwk.Name + "endpoint",
		HostComputeNetwork: ntwk.Id,
		Routes:             []hcn.Route{ipv4Route, ipv6Route},
		IpConfigurations:   []hcn.IpConfig{ip4Want, ip6Want},
		SchemaVersion:      hcn.Version{Major: 2, Minor: 2},
	}).Create()
	if err != nil {
		t.Fatalf("endpoint creation: %v", err)
	}
	t.Cleanup(func() {
		if err := ep.Delete(); err != nil {
			t.Errorf("endpoint delete: %v", err)
		}
	})
	t.Logf("created endpoint %s", ep.Id)

	for _, ip := range ep.IpConfigurations {
		if ip != ip4Want && ip != ip6Want {
			t.Fatalf("endpoint address (%v) != %v or %v", ip, ip4Want, ip6Want)
		}
		t.Logf("ip %s/%d", ip.IpAddress, ip.PrefixLength)
	}

	if err := ep.NamespaceAttach(ns.Id); err != nil {
		t.Fatalf("network attachment: %v", err)
	}

	ctx := util.Context(namespacedContext(context.Background()), t)
	ls := linuxImageLayers(ctx, t)
	opts := defaultLCOWOptions(ctx, t)
	vm := testuvm.CreateAndStartLCOWFromOpts(ctx, t, opts)

	if err := vm.CreateAndAssignNetworkSetup(ctx, "", ""); err != nil {
		t.Fatalf("setting up network: %v", err)
	}
	if err := vm.ConfigureNetworking(ctx, ns.Id); err != nil {
		t.Fatalf("adding network to vm: %v", err)
	}

	cID := strings.ReplaceAll(t.Name(), "/", "")
	scratch, _ := testlayers.ScratchSpace(ctx, t, vm, "", "", "")
	spec := testoci.CreateLinuxSpec(ctx, t, cID,
		testoci.DefaultLinuxSpecOpts(ns.Id,
			ctrdoci.WithProcessArgs("/bin/sh", "-c", testoci.TailNullArgs),
			ctrdoci.WithWindowsNetworkNamespace(ns.Id),
			testoci.WithWindowsLayerFolders(append(ls, scratch)))...)

	c, _, cleanup := testcontainer.Create(ctx, t, vm, spec, cID, hcsOwner)
	t.Logf("created container %s", cID)
	t.Cleanup(cleanup)
	init := testcontainer.Start(ctx, t, c, nil)
	t.Cleanup(func() {
		testcmd.Kill(ctx, t, init)
		testcmd.Wait(ctx, t, init)
		testcontainer.Kill(ctx, t, c)
		testcontainer.Wait(ctx, t, c)
	})

	ps := testoci.CreateLinuxSpec(ctx, t, cID,
		testoci.DefaultLinuxSpecOpts(ns.Id,
			ctrdoci.WithDefaultPathEnv,
			ctrdoci.WithProcessArgs("/bin/sh", "-c", `ip -o address show dev eth0 scope global`),
		)...,
	).Process
	io := testcmd.NewBufferedIO()
	p := testcmd.Create(ctx, t, c, ps, io)
	testcmd.Start(ctx, t, p)

	e := testcmd.Wait(ctx, t, p)
	out, err := io.Output()
	t.Logf("cmd output:\n%s", out)
	if e != 0 || err != nil {
		t.Fatalf("exit code %d and error %v", e, err)
	}

	for _, ipc := range ep.IpConfigurations {
		ip := fmt.Sprintf("%s/%d", ipc.IpAddress, ipc.PrefixLength)
		if !strings.Contains(out, ip) {
			t.Errorf("missing ip address %s", ip)
		}
	}
}

func newNetworkNamespace() (*hcn.HostComputeNamespace, error) {
	return (&hcn.HostComputeNamespace{}).Create()
}
