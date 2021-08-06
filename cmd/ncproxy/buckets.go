package main

import (
	bolt "go.etcd.io/bbolt"
)

const schemaVersion = "v1"

var (
	bucketKeyVersion      = []byte(schemaVersion)
	bucketKeyComputeAgent = []byte("computeagent")
)

// taken from containerd/containerd/metadata/buckets.go
func getBucket(tx *bolt.Tx, keys ...[]byte) *bolt.Bucket {
	bkt := tx.Bucket(keys[0])

	for _, key := range keys[1:] {
		if bkt == nil {
			break
		}
		bkt = bkt.Bucket(key)
	}

	return bkt
}

// taken from containerd/containerd/metadata/buckets.go
func createBucketIfNotExists(tx *bolt.Tx, keys ...[]byte) (*bolt.Bucket, error) {
	bkt, err := tx.CreateBucketIfNotExists(keys[0])
	if err != nil {
		return nil, err
	}

	for _, key := range keys[1:] {
		bkt, err = bkt.CreateBucketIfNotExists(key)
		if err != nil {
			return nil, err
		}
	}

	return bkt, nil
}

func createComputeAgentBucket(tx *bolt.Tx) (*bolt.Bucket, error) {
	return createBucketIfNotExists(tx, bucketKeyVersion, bucketKeyComputeAgent)
}

func getComputeAgentBucket(tx *bolt.Tx) *bolt.Bucket {
	return getBucket(tx, bucketKeyVersion, bucketKeyComputeAgent)
}
