package seth

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/core/types"
)

type cacheItem struct {
	block     *types.Block
	frequency int
}

// LFUBlockCache is a Least Frequently Used block cache
type LFUBlockCache struct {
	capacity uint64
	mu       sync.Mutex
	cache    map[int64]*cacheItem //key is block number
}

// NewLFUBlockCache creates a new LFU cache with the given capacity.
func NewLFUBlockCache(capacity uint64) *LFUBlockCache {
	return &LFUBlockCache{
		capacity: capacity,
		cache:    make(map[int64]*cacheItem),
	}
}

// Get retrieves a block from the cache.
func (c *LFUBlockCache) Get(blockNumber int64) (*types.Block, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, found := c.cache[blockNumber]; found {
		item.frequency++
		L.Trace().Msgf("Found block %d in cache", blockNumber)
		return item.block, true
	}
	return nil, false
}

// Set adds or updates a block in the cache.
func (c *LFUBlockCache) Set(block *types.Block) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if oldBlock, found := c.cache[int64(block.NumberU64())]; found {
		L.Trace().Msgf("Setting block %d in cache", block.NumberU64())
		c.cache[int64(block.NumberU64())] = &cacheItem{block: block, frequency: oldBlock.frequency + 1}
		return nil
	}

	if uint64(len(c.cache)) >= c.capacity {
		c.evict()
	}
	L.Trace().Msgf("Setting block %d in cache", block.NumberU64())
	c.cache[int64(block.NumberU64())] = &cacheItem{block: block, frequency: 1}

	return nil
}

// evict removes the least frequently used item from the cache. If more than one item has the same frequency, the oldest is evicted.
func (c *LFUBlockCache) evict() {
	var leastFreq int = int(^uint(0) >> 1)
	var evictKey int64
	oldestBlockNumber := ^uint64(0)
	for key, item := range c.cache {
		if item.frequency < leastFreq {
			evictKey = key
			leastFreq = item.frequency
			oldestBlockNumber = item.block.NumberU64()
		} else if item.frequency == leastFreq && item.block.NumberU64() < oldestBlockNumber {
			// If frequencies are the same, evict the oldest based on block number
			evictKey = key
			oldestBlockNumber = item.block.NumberU64()
		}
	}
	L.Trace().Msgf("Evicted block %d from cache", evictKey)
	delete(c.cache, evictKey)
}
