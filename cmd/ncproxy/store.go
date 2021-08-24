package main

import (
	"context"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

var (
	errBucketNotFound = errors.New("bucket not found")
	errKeyNotFound    = errors.New("key does not exist")
)

// computeAgentStore is a database that stores a key value pair of container id
// to compute agent server address
type computeAgentStore struct {
	db *bolt.DB
}

func newComputeAgentStore(db *bolt.DB) *computeAgentStore {
	return &computeAgentStore{db: db}
}

func (c *computeAgentStore) Close() error {
	return c.db.Close()
}

// getComputeAgents returns a map of the key value pairs stored in the database
// where the keys are the containerIDs and the values are the corresponding compute agent
// server addresses
func (c *computeAgentStore) getComputeAgents(ctx context.Context) (map[string]string, error) {
	content := map[string]string{}
	if err := c.db.View(func(tx *bolt.Tx) error {
		bkt := getComputeAgentBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errBucketNotFound, "bucket %v", bucketKeyComputeAgent)
		}
		err := bkt.ForEach(func(k, v []byte) error {
			data := bkt.Get([]byte(k))
			content[string(k)] = string(data)
			return nil
		})
		return err
	}); err != nil {
		return nil, err
	}
	return content, nil
}

// updateComputeAgent updates or adds an entry (if none already exists) to the database
// `address` corresponds to the address of the compute agent server for the `containerID`
func (c *computeAgentStore) updateComputeAgent(ctx context.Context, containerID string, address string) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bkt, err := createComputeAgentBucket(tx)
		if err != nil {
			return err
		}
		return bkt.Put([]byte(containerID), []byte(address))
	}); err != nil {
		return err
	}
	return nil
}

// deleteComputeAgent deletes an entry in the database or returns an error if none exists
// `containerID` corresponds to the target key that the entry should be deleted for
func (c *computeAgentStore) deleteComputeAgent(ctx context.Context, containerID string) error {
	if err := c.db.Update(func(tx *bolt.Tx) error {
		bkt := getComputeAgentBucket(tx)
		if bkt == nil {
			return errors.Wrapf(errBucketNotFound, "bucket %v", bucketKeyComputeAgent)
		}
		return bkt.Delete([]byte(containerID))
	}); err != nil {
		return err
	}
	return nil
}
