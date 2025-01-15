//go:build windows

package uvm

import (
	"fmt"
	"testing"

	"github.com/Microsoft/hcsshim/hcn"
)

func Test_SortEndpoints(t *testing.T) {
	type config struct {
		endpointNames []string
		targetName    string
	}
	tests := []config{
		{
			endpointNames: []string{"eth1", "eth0"},
			targetName:    "eth0",
		},
		{
			endpointNames: []string{"eth0", "eth1"},
			targetName:    "eth0",
		},
		{
			endpointNames: []string{"eth1", "name-eth0"},
			targetName:    "name-eth0",
		},
		{
			endpointNames: []string{"name-eth098", "name-eth0"},
			targetName:    "name-eth0",
		},
		{
			endpointNames: []string{"name-eth0", "name-eth098"},
			targetName:    "name-eth0",
		},
		{
			endpointNames: []string{"random-ifname", "another-random-ifname"},
			// ordering shouldn't change so the first entry should still be first
			targetName: "random-ifname",
		},
		{
			endpointNames: []string{"eth0-name", "name-eth0"},
			targetName:    "name-eth0",
		},
		{
			endpointNames: []string{},
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprint(t.Name(), i), func(st *testing.T) {
			endpoints := []*hcn.HostComputeEndpoint{}
			for _, n := range test.endpointNames {
				e := &hcn.HostComputeEndpoint{
					Name: n,
				}
				endpoints = append(endpoints, e)
			}

			sortEndpoints(endpoints)
			if len(test.endpointNames) != 0 {
				if endpoints[0].Name != test.targetName {
					st.Fatalf("expected endpoint sorting to return endpoint with name %s first, instead got %s", test.targetName, endpoints[0].Name)
				}
			}
		})
	}
}
