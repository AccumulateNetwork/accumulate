// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"
)

func TestURLHashFunctionality(t *testing.T) {
	// Create a new SnapCombine instance
	sc := &SnapCombine{}
	
	// Initialize with memory mode enabled for testing
	sc.useMemory = true
	sc.urlHashMap = make(map[[32]byte]string)
	
	// Test URLs
	urls := []string{
		"acc://example.acme/main",
		"acc://test.acme/scratch",
		"acc://long-url-with-many-characters.acme/special-chain",
	}
	
	// Store each URL and verify we can retrieve it
	for _, url := range urls {
		// Store the URL hash
		urlHash, err := sc.StoreURLHash(url)
		if err != nil {
			t.Fatalf("Failed to store URL hash for %s: %v", url, err)
		}
		
		// Retrieve the URL from the hash
		retrievedURL, err := sc.GetURLFromHash(urlHash)
		if err != nil {
			t.Fatalf("Failed to retrieve URL for hash: %v", err)
		}
		
		// Verify the retrieved URL matches the original
		if retrievedURL != url {
			t.Errorf("URL mismatch: expected %s, got %s", url, retrievedURL)
		}
	}
}

func TestURLHashFileStorage(t *testing.T) {
	// Create a new SnapCombine instance
	sc := &SnapCombine{}
	
	// Initialize with file mode for testing
	if err := sc.Initialize(); err != nil {
		t.Fatalf("Failed to initialize SnapCombine: %v", err)
	}
	defer sc.Cleanup()
	
	// Force file mode
	sc.useMemory = false
	
	// Test URLs
	urls := []string{
		"acc://example.acme/main",
		"acc://test.acme/scratch",
		"acc://long-url-with-many-characters.acme/special-chain",
	}
	
	// Store each URL
	urlHashes := make([][32]byte, len(urls))
	for i, url := range urls {
		// Store the URL hash
		urlHash, err := sc.StoreURLHash(url)
		if err != nil {
			t.Fatalf("Failed to store URL hash for %s: %v", url, err)
		}
		urlHashes[i] = urlHash
	}
	
	// Load the URL hash map into memory
	if err := sc.LoadURLHashMap(); err != nil {
		t.Fatalf("Failed to load URL hash map: %v", err)
	}
	
	// Verify each URL can be retrieved from the map
	for i, url := range urls {
		// Get the URL from the hash using the in-memory map
		retrievedURL, ok := sc.urlHashMap[urlHashes[i]]
		if !ok {
			t.Errorf("URL hash not found in map for %s", url)
			continue
		}
		
		// Verify the retrieved URL matches the original
		if retrievedURL != url {
			t.Errorf("URL mismatch: expected %s, got %s", url, retrievedURL)
		}
	}
}
