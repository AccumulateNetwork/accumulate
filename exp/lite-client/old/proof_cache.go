package liteclient

import (
	"container/list"
	"sync"
)

// ProofCache provides thread-safe caching of AccountProofs
// using an LRU eviction policy when capacity is reached.
type ProofCache struct {
	mu    sync.RWMutex
	store map[string]*list.Element
	lru   *list.List
	max   int
}

// cacheEntry represents a single cached proof
// combining the lookup key and proof value
type cacheEntry struct {
	key   string
	proof *AccountProof
}

// NewProofCache creates a new cache with the specified maximum capacity
func NewProofCache(maxEntries int) *ProofCache {
	return &ProofCache{
		store: make(map[string]*list.Element),
		lru:   list.New(),
		max:   maxEntries,
	}
}

// Get retrieves a proof from cache if it exists and moves it to front of LRU
func (c *ProofCache) Get(key string) (*AccountProof, bool) {
	c.mu.RLock()
	elem, ok := c.store[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	c.mu.Lock()
	c.lru.MoveToFront(elem)
	c.mu.Unlock()
	return elem.Value.(*cacheEntry).proof, true
}

// Set adds a proof to cache, evicting oldest if at capacity
func (c *ProofCache) Set(key string, proof *AccountProof) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If exists, update and move to front
	if elem, ok := c.store[key]; ok {
		elem.Value.(*cacheEntry).proof = proof
		c.lru.MoveToFront(elem)
		return
	}

	// Add new entry
	elem := c.lru.PushFront(&cacheEntry{key, proof})
	c.store[key] = elem

	// Evict if needed
	if c.lru.Len() > c.max {
		oldest := c.lru.Back()
		if oldest != nil {
			delete(c.store, oldest.Value.(*cacheEntry).key)
			c.lru.Remove(oldest)
		}
	}
}

// Invalidate removes a proof from cache
func (c *ProofCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.store[key]; ok {
		delete(c.store, key)
		c.lru.Remove(elem)
	}
}

// Size returns current number of cached proofs
func (c *ProofCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}
