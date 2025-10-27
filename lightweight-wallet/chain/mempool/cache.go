package mempool

import (
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// cache provides a simple in-memory cache with TTL.
type cache struct {
	// Current height cache
	height       uint32
	heightExpiry time.Time

	// Block hash cache (height -> hash)
	blockHashes map[uint32]cacheEntry

	// Block timestamp cache (height -> timestamp)
	blockTimestamps map[uint32]cacheEntry

	ttl time.Duration
	mu  sync.RWMutex
}

// newCache creates a new cache.
func newCache(size int, ttl time.Duration) *cache {
	return &cache{
		blockHashes:     make(map[uint32]cacheEntry, size),
		blockTimestamps: make(map[uint32]cacheEntry, size),
		ttl:             ttl,
	}
}

// getHeight returns the cached height if valid.
func (c *cache) getHeight() (uint32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if time.Now().Before(c.heightExpiry) && c.height > 0 {
		return c.height, true
	}

	return 0, false
}

// setHeight caches the current height.
func (c *cache) setHeight(height uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.height = height
	c.heightExpiry = time.Now().Add(c.ttl)
}

// getBlockHash returns the cached block hash if valid.
func (c *cache) getBlockHash(height uint32) (chainhash.Hash, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.blockHashes[height]
	if !ok {
		return chainhash.Hash{}, false
	}

	if time.Now().After(entry.expiresAt) {
		return chainhash.Hash{}, false
	}

	hash, ok := entry.value.(chainhash.Hash)
	return hash, ok
}

// setBlockHash caches a block hash.
func (c *cache) setBlockHash(height uint32, hash chainhash.Hash) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.blockHashes[height] = cacheEntry{
		value:     hash,
		expiresAt: time.Now().Add(c.ttl),
	}

	// Simple LRU: remove oldest entries if cache is too large
	if len(c.blockHashes) > 100 {
		// Find and remove oldest entry
		var oldestHeight uint32
		oldestTime := time.Now()
		for h, entry := range c.blockHashes {
			if entry.expiresAt.Before(oldestTime) {
				oldestTime = entry.expiresAt
				oldestHeight = h
			}
		}
		delete(c.blockHashes, oldestHeight)
	}
}

// getBlockTimestamp returns the cached block timestamp if valid.
func (c *cache) getBlockTimestamp(height uint32) (int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.blockTimestamps[height]
	if !ok {
		return 0, false
	}

	if time.Now().After(entry.expiresAt) {
		return 0, false
	}

	timestamp, ok := entry.value.(int64)
	return timestamp, ok
}

// setBlockTimestamp caches a block timestamp.
func (c *cache) setBlockTimestamp(height uint32, timestamp int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.blockTimestamps[height] = cacheEntry{
		value:     timestamp,
		expiresAt: time.Now().Add(c.ttl),
	}

	// Simple LRU: remove oldest entries if cache is too large
	if len(c.blockTimestamps) > 1000 {
		// Find and remove oldest entry
		var oldestHeight uint32
		oldestTime := time.Now()
		for h, entry := range c.blockTimestamps {
			if entry.expiresAt.Before(oldestTime) {
				oldestTime = entry.expiresAt
				oldestHeight = h
			}
		}
		delete(c.blockTimestamps, oldestHeight)
	}
}

// cleanup removes expired entries from the cache.
func (c *cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Clean up block hashes
	for height, entry := range c.blockHashes {
		if now.After(entry.expiresAt) {
			delete(c.blockHashes, height)
		}
	}

	// Clean up block timestamps
	for height, entry := range c.blockTimestamps {
		if now.After(entry.expiresAt) {
			delete(c.blockTimestamps, height)
		}
	}
}
