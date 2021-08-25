package ncproxy

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Microsoft/hcsshim/cmd/ncproxy/ncproxygrpc"
	bolt "go.etcd.io/bbolt"
)

func TestEndpointStore_CustomEndpoint(t *testing.T) {
	ctx := context.Background()
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := bolt.Open(filepath.Join(tempDir, "endpoint.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewEndpointStore(db)
	endptName := "test-endpoint"
	endpt := &customEndpoint{
		Name: endptName,
	}

	// put endpoint in the database
	if err := store.Update(ctx, endptName, endpt); err != nil {
		t.Fatal(err)
	}

	// retrieve the endpoint from the database
	endptActual, err := store.Get(ctx, endptName)
	if err != nil {
		t.Fatal(err)
	}
	if endptActual == nil {
		t.Fatalf("expected to get %v, instead got nil endpoint", endpt)
	}
	if !reflect.DeepEqual(endpt, endptActual) {
		t.Fatalf("endpoints are not equal, expected %v but got %v", endpt, endptActual)
	}
}

func TestEndpointStore_HcnEndpoint(t *testing.T) {
	ctx := context.Background()
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := bolt.Open(filepath.Join(tempDir, "endpoint.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewEndpointStore(db)
	endptName := "test-endpoint"
	endpt := &hcnEndpoint{
		Name: endptName,
		Settings: &ncproxygrpc.EndpointSettings{
			DnsSetting: &ncproxygrpc.DnsSetting{
				ServerIpAddrs: []string{"a", "b", "c"},
				Domain:        "test-domain",
			},
		},
	}

	// put endpoint in the database
	if err := store.Update(ctx, endptName, endpt); err != nil {
		t.Fatal(err)
	}

	// retrieve the endpoint from the database
	endptActual, err := store.Get(ctx, endptName)
	if err != nil {
		t.Fatal(err)
	}
	if endptActual == nil {
		t.Fatalf("expected to get %v, instead got nil endpoint", endpt)
	}
	if !reflect.DeepEqual(endpt, endptActual) {
		t.Fatalf("endpoints are not equal, expected %v but got %v", endpt, endptActual)
	}
}
