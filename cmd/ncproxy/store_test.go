package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// getComputeAgent returns the compute agent address of a single entry in the database for key `containerID`
// or returns an error if the key does not exist
func (c *computeAgentStore) getComputeAgent(ctx context.Context, containerID string) (result string, err error) {
	if err := c.db.View(func(tx *bolt.Tx) error {
		bkt := getComputeAgentBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errBucketNotFound, "bucket %v", bucketKeyComputeAgent)
		}
		data := bkt.Get([]byte(containerID))
		if data == nil {
			return errors.Wrapf(errKeyNotFound, "key %v", containerID)
		}
		result = string(data)
		return nil
	}); err != nil {
		return "", err
	}

	return result, nil
}

func TestComputeAgentStore(t *testing.T) {
	ctx := context.Background()
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := newComputeAgentStore(db)
	containerID := "fake-container-id"
	address := "123412341234"

	if err := store.updateComputeAgent(ctx, containerID, address); err != nil {
		t.Fatal(err)
	}

	actual, err := store.getComputeAgent(ctx, containerID)
	if err != nil {
		t.Fatal(err)
	}

	if address != actual {
		t.Fatalf("compute agent addresses are not equal, expected %v but got %v", address, actual)
	}

	if err := store.deleteComputeAgent(ctx, containerID); err != nil {
		t.Fatal(err)
	}

	value, err := store.getComputeAgent(ctx, containerID)
	if err == nil {
		t.Fatalf("expected an error, instead found value %s", value)
	}
}

func TestComputeAgentStore_GetKeyValueMap(t *testing.T) {
	ctx := context.Background()
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := bolt.Open(filepath.Join(tempDir, "networkproxy.db.test"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := newComputeAgentStore(db)

	containerIDs := []string{"fake-container-id", "fake-container-id-2"}
	addresses := []string{"123412341234", "234523452345"}

	target := make(map[string]string)
	for i := 0; i < len(containerIDs); i++ {
		target[containerIDs[i]] = addresses[i]
		if err := store.updateComputeAgent(ctx, containerIDs[i], addresses[i]); err != nil {
			t.Fatal(err)
		}
	}

	actual, err := store.getComputeAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range actual {
		if target[k] != v {
			t.Fatalf("expected to get %s for key %s, instead got %s", target[k], k, v)
		}
	}
}
