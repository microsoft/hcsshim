package ncproxy

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestNetworkStore(t *testing.T) {
	ctx := context.Background()
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := bolt.Open(filepath.Join(tempDir, "network.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewNetworkStore(db)
	networkName := "test-network"
	network := &hcnNetwork{
		NetworkName: networkName,
		Id:          "1234-1234-12341234-1234",
	}

	// put network in the database
	if err := store.Update(ctx, networkName, network); err != nil {
		t.Fatal(err)
	}

	// retrieve the network from the database
	networkActual, err := store.Get(ctx, networkName)
	if err != nil {
		t.Fatal(err)
	}
	if networkActual == nil {
		t.Fatalf("expected to get %v, instead got nil network", network)
	}
	if !reflect.DeepEqual(network, networkActual) {
		t.Fatalf("networks are not equal, expected %v but got %v", network, networkActual)
	}
}
