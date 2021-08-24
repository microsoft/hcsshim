package main

import (
	bolt "go.etcd.io/bbolt"
)

const schemaVersion = "v1"

var (
	bucketKeyVersion      = []byte(schemaVersion)
	bucketKeyComputeAgent = []byte("computeagent")
)

// Below is the current database schema. This should be updated any time the schema is
// changed or updated. The version should be incremented if breaking changes are made.
//  └──v1                                        - Schema version bucket
//     └──computeagent
//			└──containerID : <string>            - Address to the compute agent for containerID

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
