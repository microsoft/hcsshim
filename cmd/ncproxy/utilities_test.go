package main

import (
	"os"
	"path/filepath"

	ncproxystore "github.com/Microsoft/hcsshim/internal/ncproxy/store"
	ncproxygrpc "github.com/Microsoft/hcsshim/pkg/ncproxy/ncproxygrpc/v1"
	bolt "go.etcd.io/bbolt"
)

func exists(target string, list []string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func networkExists(targetName string, networks []*ncproxygrpc.GetNetworkResponse) bool {
	for _, resp := range networks {
		n := resp.Network.Settings
		switch network := n.(type) {
		case *ncproxygrpc.Network_HcnNetwork:
			if network.HcnNetwork != nil && network.HcnNetwork.Name == targetName {
				return true
			}
		case *ncproxygrpc.Network_NcproxyNetwork:
			if network.NcproxyNetwork != nil && network.NcproxyNetwork.Name == targetName {
				return true
			}
		}
	}
	return false
}

func endpointExists(targetName string, endpoints []*ncproxygrpc.GetEndpointResponse) bool {
	for _, resp := range endpoints {
		ep := resp.Endpoint.Settings
		switch endpt := ep.(type) {
		case *ncproxygrpc.EndpointSettings_HcnEndpoint:
			if endpt.HcnEndpoint != nil && endpt.HcnEndpoint.Name == targetName {
				return true
			}
		case *ncproxygrpc.EndpointSettings_NcproxyEndpoint:
			if resp.ID == targetName {
				return true
			}
		}
	}
	return false
}

func createTestNetworkingStore() (store *ncproxystore.NetworkingStore, closer func(), err error) {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, nil, err
	}

	defer func() {
		if err != nil {
			_ = os.RemoveAll(tempDir)
		}
	}()

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		return nil, nil, err
	}

	closer = func() {
		_ = os.RemoveAll(tempDir)
		_ = db.Close()
	}

	return ncproxystore.NewNetworkingStore(db), closer, nil
}
