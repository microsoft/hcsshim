package main

import (
	"sync"

	"github.com/pkg/errors"
)

var errNilCache = errors.New("cannot access a nil cache")

type computeAgentCache struct {
	// lock for synchronizing read/write access to `cache`
	rw sync.RWMutex
	// mapping of container ID to shim compute agent ttrpc service
	cache map[string]*computeAgentClient
}

func newComputeAgentCache() *computeAgentCache {
	return &computeAgentCache{
		cache: make(map[string]*computeAgentClient),
	}
}

func (c *computeAgentCache) getAllAndClear() ([]*computeAgentClient, error) {
	// set c.cache to nil first so that subsequent attempts to reads and writes
	// return an error
	c.rw.Lock()
	cacheCopy := c.cache
	c.cache = nil
	c.rw.Unlock()

	if cacheCopy == nil {
		return nil, errNilCache
	}

	results := []*computeAgentClient{}
	for _, agent := range cacheCopy {
		results = append(results, agent)
	}
	return results, nil

}

func (c *computeAgentCache) get(cid string) (*computeAgentClient, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	if c.cache == nil {
		return nil, errNilCache
	}
	result := c.cache[cid]
	return result, nil
}

func (c *computeAgentCache) put(cid string, agent *computeAgentClient) error {
	c.rw.Lock()
	defer c.rw.Unlock()
	if c.cache == nil {
		return errNilCache
	}
	c.cache[cid] = agent
	return nil
}

func (c *computeAgentCache) getAndDelete(cid string) (*computeAgentClient, error) {
	c.rw.Lock()
	defer c.rw.Unlock()
	if c.cache == nil {
		return nil, errNilCache
	}
	result := c.cache[cid]
	delete(c.cache, cid)
	return result, nil
}
