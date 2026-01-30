//go:build linux
// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Microsoft/hcsshim/internal/protocol/guestresource"
)

func Test_getNetworkNamespace_NotExist(t *testing.T) {
	defer func() {
		err := RemoveNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns, err := getNetworkNamespace(t.Name())
	if err == nil {
		t.Fatal("expected error got nil")
	}
	if ns != nil {
		t.Fatalf("namespace should be nil, got: %+v", ns)
	}
}

func Test_getNetworkNamespace_PreviousExist(t *testing.T) {
	defer func() {
		err := RemoveNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns1 := GetOrAddNetworkNamespace(t.Name())
	if ns1 == nil {
		t.Fatal("namespace ns1 should not be nil")
	}
	ns2, err := getNetworkNamespace(t.Name())
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
	if ns1 != ns2 {
		t.Fatalf("ns1 %+v != ns2 %+v", ns1, ns2)
	}
}

func Test_getOrAddNetworkNamespace_NotExist(t *testing.T) {
	defer func() {
		err := RemoveNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns := GetOrAddNetworkNamespace(t.Name())
	if ns == nil {
		t.Fatalf("namespace should not be nil")
	}
}

func Test_getOrAddNetworkNamespace_PreviousExist(t *testing.T) {
	defer func() {
		err := RemoveNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns1 := GetOrAddNetworkNamespace(t.Name())
	ns2 := GetOrAddNetworkNamespace(t.Name())
	if ns1 != ns2 {
		t.Fatalf("ns1 %+v != ns2 %+v", ns1, ns2)
	}
}

func Test_removeNetworkNamespace_NotExist(t *testing.T) {
	err := RemoveNetworkNamespace(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("failed to remove non-existing ns with error: %v", err)
	}
}

func Test_removeNetworkNamespace_HasAdapters(t *testing.T) {
	defer func() {
		err := RemoveNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()
	nsOld := networkInstanceIDToName
	defer func() {
		networkInstanceIDToName = nsOld
	}()

	ns := GetOrAddNetworkNamespace(t.Name())

	networkInstanceIDToName = func(_ context.Context, _ string, _ bool) (string, error) {
		return "/dev/sdz", nil
	}
	err := ns.AddAdapter(context.Background(), &guestresource.LCOWNetworkAdapter{ID: "test"})
	if err != nil {
		t.Fatalf("failed to add adapter: %v", err)
	}
	err = RemoveNetworkNamespace(context.Background(), t.Name())
	if err == nil {
		t.Fatal("should have failed to delete namespace with adapters")
	}
	err = ns.RemoveAdapter(context.Background(), "test")
	if err != nil {
		t.Fatalf("failed to remove adapter: %v", err)
	}
	err = RemoveNetworkNamespace(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("should not have failed to delete empty namepace got: %v", err)
	}
}

func TestDNSConfig(t *testing.T) {
	t.Cleanup(func() {
		err := RemoveNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	})

	nsOld := networkInstanceIDToName
	networkInstanceIDToName = func(_ context.Context, _ string, _ bool) (string, error) {
		return "/dev/sdz", nil
	}
	t.Cleanup(func() {
		networkInstanceIDToName = nsOld
	})

	ctx := t.Context()
	ns := GetOrAddNetworkNamespace(t.Name())

	for i, tc := range []struct {
		searches     string
		servers      string
		wantSearches []string
		wantServers  []string
	}{
		{},
		{
			servers:     "1.1.1.1",
			wantServers: []string{"1.1.1.1"},
		},
		{
			searches:     "azure-dns.com",
			wantSearches: []string{"azure-dns.com"},
			wantServers:  []string{"1.1.1.1"},
		},
		{
			searches: strings.Join([]string{
				"Azure-DNS.com",
				"service.svc.cluster.local",
				"svc.cluster.local",
				"cluster.local",
			}, ","),
			servers: "10.11.12.13, 1.1.1.1, 8.8.8.8",
			wantSearches: []string{
				"azure-dns.com",
				"service.svc.cluster.local",
				"svc.cluster.local",
				"cluster.local",
			},
			wantServers: []string{
				"1.1.1.1",
				"10.11.12.13",
				"8.8.8.8",
			},
		},
		{
			wantSearches: []string{
				"azure-dns.com",
				"service.svc.cluster.local",
				"svc.cluster.local",
				"cluster.local",
			},
			wantServers: []string{
				"1.1.1.1",
				"10.11.12.13",
				"8.8.8.8",
			},
		},
		{
			servers: "FC00::A",
			wantSearches: []string{
				"azure-dns.com",
				"service.svc.cluster.local",
				"svc.cluster.local",
				"cluster.local",
			},
			wantServers: []string{
				"1.1.1.1",
				"10.11.12.13",
				"8.8.8.8",
				"fc00::a",
			},
		},
	} {
		id := fmt.Sprintf("test-nic%d", i)
		t.Logf("adding NIC %d: %s", i+1, id)
		t.Logf("searches: %q", tc.searches)
		t.Logf("servers:  %q", tc.servers)

		if err := ns.AddAdapter(ctx, &guestresource.LCOWNetworkAdapter{
			ID:            id,
			DNSSuffix:     tc.searches,
			DNSServerList: tc.servers,
		}); err != nil {
			t.Fatalf("failed to add adapter: %v", err)
		}
		t.Cleanup(func() {
			if err := ns.RemoveAdapter(ctx, id); err != nil {
				t.Errorf("failed to remove adapter: %v", err)
			}
		})

		searches, servers := ns.dnsConfig(ctx)
		if diff := cmp.Diff(tc.wantSearches, searches, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("DNS searches mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(tc.wantServers, servers, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("DNS servers mismatch (-want +got):\n%s", diff)
		}

		if t.Failed() {
			// don't keep failing if one of the DNS config arrays is invalid
			break
		}
	}

}
