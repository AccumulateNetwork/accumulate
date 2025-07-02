// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"sync"
)

// ChainEntryCache caches chain entries to avoid repeated extraction
type ChainEntryCache struct {
	entries map[string][][]byte
	mutex  sync.RWMutex
}

// NewChainEntryCache creates a new chain entry cache
func NewChainEntryCache() *ChainEntryCache {
	return &ChainEntryCache{
		entries: make(map[string][][]byte),
	}
}

// Get retrieves chain entries from the cache
func (c *ChainEntryCache) Get(chainURL string) ([][]byte, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	entries, ok := c.entries[chainURL]
	return entries, ok
}

// Set stores chain entries in the cache
func (c *ChainEntryCache) Set(chainURL string, entries [][]byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.entries[chainURL] = entries
}

// getChainEntries extracts chain entries from the snapshot records
func getChainEntries(extractState *ExtractState, chainURL string) ([][]byte, error) {
	// Check cache first
	if extractState.ChainEntryCache == nil {
		extractState.ChainEntryCache = NewChainEntryCache()
	}
	
	entries, found := extractState.ChainEntryCache.Get(chainURL)
	if found {
		return entries, nil
	}
	
	// Find the chain record
	var chainRecord *ChainRecord
	for _, record := range extractState.Records {
		if record.Type != "chain" {
			continue
		}
		
		if record.URL == chainURL {
			chainRecord = record.Chain
			break
		}
	}
	
	if chainRecord == nil {
		// Chain not found, return empty slice
		return [][]byte{}, nil
	}
	
	// Extract entry hashes from the chain
	entries = make([][]byte, 0, len(chainRecord.Entries))
	for _, entry := range chainRecord.Entries {
		// Add the entry hash
		entries = append(entries, entry.Hash)
	}
	
	// Cache the results
	extractState.ChainEntryCache.Set(chainURL, entries)
	
	return entries, nil
}
