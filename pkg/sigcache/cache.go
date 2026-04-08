// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sigcache

import (
	"crypto/sha256"
	"sync"
	"time"
)

// Config contains configuration for the signature cache.
type Config struct {
	// MaxEntries is the maximum number of entries to cache (default 10,000)
	MaxEntries int

	// TTL is the time-to-live for cached entries (default 300s)
	TTL time.Duration

	// Enabled determines if caching is enabled (default true)
	Enabled bool
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		MaxEntries: 10000,
		TTL:        300 * time.Second,
		Enabled:    true,
	}
}

// Validate validates the cache configuration.
func (c *Config) Validate() error {
	if c.MaxEntries < 0 {
		c.MaxEntries = 0
	}
	if c.TTL < 0 {
		c.TTL = 0
	}
	return nil
}

// ApplyDefaults fills in default values for any unset fields.
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	if c.MaxEntries <= 0 {
		c.MaxEntries = defaults.MaxEntries
	}
	if c.TTL <= 0 {
		c.TTL = defaults.TTL
	}
}

// Cache is a thread-safe LRU cache for signature verification results.
type Cache struct {
	config  *Config
	mu      sync.RWMutex
	entries map[[32]byte]*entry
	head    *entry
	tail    *entry
	size    int

	// Metrics
	hits   uint64
	misses uint64
}

type entry struct {
	key       [32]byte
	valid     bool
	timestamp time.Time
	prev      *entry
	next      *entry
}

// New creates a new signature verification cache.
func New(config *Config) *Cache {
	if config == nil {
		config = DefaultConfig()
	}
	config.ApplyDefaults()

	return &Cache{
		config:  config,
		entries: make(map[[32]byte]*entry),
	}
}

// computeKey computes the cache key from signature data, signature bytes, and public key.
func computeKey(data, signature, publicKey []byte) [32]byte {
	h := sha256.New()
	h.Write(data)
	h.Write(signature)
	h.Write(publicKey)
	return sha256.Sum256(h.Sum(nil))
}

// Check checks if a signature verification result is cached and valid.
// Returns (result, found).
func (c *Cache) Check(data, signature, publicKey []byte) (bool, bool) {
	if !c.config.Enabled {
		return false, false
	}

	key := computeKey(data, signature, publicKey)

	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok {
		c.misses++
		return false, false
	}

	// Check TTL
	if c.config.TTL > 0 && time.Since(e.timestamp) > c.config.TTL {
		c.misses++
		return false, false
	}

	c.hits++
	return e.valid, true
}

// Store stores a signature verification result in the cache.
func (c *Cache) Store(data, signature, publicKey []byte, valid bool) {
	if !c.config.Enabled {
		return
	}

	key := computeKey(data, signature, publicKey)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists
	if e, ok := c.entries[key]; ok {
		// Update existing entry and move to front
		e.valid = valid
		e.timestamp = time.Now()
		c.moveToFront(e)
		return
	}

	// Create new entry
	e := &entry{
		key:       key,
		valid:     valid,
		timestamp: time.Now(),
	}

	// Add to map
	c.entries[key] = e

	// Add to front of list
	c.addToFront(e)
	c.size++

	// Evict if over capacity
	if c.config.MaxEntries > 0 && c.size > c.config.MaxEntries {
		c.evictOldest()
	}
}

// addToFront adds an entry to the front of the LRU list.
func (c *Cache) addToFront(e *entry) {
	e.next = c.head
	e.prev = nil

	if c.head != nil {
		c.head.prev = e
	}
	c.head = e

	if c.tail == nil {
		c.tail = e
	}
}

// moveToFront moves an existing entry to the front of the LRU list.
func (c *Cache) moveToFront(e *entry) {
	if e == c.head {
		return
	}

	// Remove from current position
	if e.prev != nil {
		e.prev.next = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	}
	if e == c.tail {
		c.tail = e.prev
	}

	// Add to front
	e.next = c.head
	e.prev = nil
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
}

// evictOldest removes the oldest entry from the cache.
func (c *Cache) evictOldest() {
	if c.tail == nil {
		return
	}

	// Remove from map
	delete(c.entries, c.tail.key)

	// Remove from list
	if c.tail.prev != nil {
		c.tail.prev.next = nil
	}
	c.tail = c.tail.prev

	if c.tail == nil {
		c.head = nil
	}

	c.size--
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[[32]byte]*entry)
	c.head = nil
	c.tail = nil
	c.size = 0
}

// Metrics returns cache hit and miss counts.
func (c *Cache) Metrics() (hits, misses uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

// HitRate returns the cache hit rate as a percentage.
func (c *Cache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total) * 100
}

// Size returns the current number of entries in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.size
}

// ResetMetrics resets hit and miss counters.
func (c *Cache) ResetMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits = 0
	c.misses = 0
}
