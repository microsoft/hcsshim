package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

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

	actual, err := store.getActiveComputeAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for k, v := range actual {
		if target[k] != v {
			t.Fatalf("expected to get %s for key %s, instead got %s", target[k], k, v)
		}
	}
}
