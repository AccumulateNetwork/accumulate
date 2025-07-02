package main

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestBloomFilterWithMillionsOfHashes(t *testing.T) {
	// Create a new bloom filter
	bloom := NewBloom("test-partition")

	// Start timer
	startTime := time.Now()

	// Generate and add 1 million SHA256 hashes for faster testing
	const numHashes = 20_000_000

	// Initial hash
	hash := sha256.Sum256([]byte{1, 2, 3, 4})

	// Add hashes to bloom filter
	for i := 0; i < numHashes; i++ {
		// Add the current hash to the bloom filter
		bloom.Add(hash[:])

		// Calculate the next hash (hash of the current hash)
		hash = sha256.Sum256(hash[:])

		// Print progress every million hashes
		if i > 0 && i%20_000_000 == 0 {
			fmt.Printf("Added %d million hashes (%.2f%%)\n", i/1_000_000, float64(i)/float64(numHashes)*100)
		}
	}

	// Record build time
	bloom.Stats.BuildTime = time.Since(startTime)

	// Print statistics
	fmt.Println(bloom.GetStats())

	// Test some known hashes (should be true)
	hash = sha256.Sum256([]byte{1, 2, 3, 4})

	// Test the first 100 hashes (should all be true)
	for i := 0; i < 100; i++ {
		if !bloom.Test(hash[:]) {
			t.Errorf("Hash %d should be in the bloom filter but was not found", i)
		}

		// Calculate the next hash
		hash = sha256.Sum256([]byte{1, 2, 3, 4})

	}

	// Test some random hashes (likely false)
	hash = sha256.Sum256([]byte{1, 2, 3})
	falsePositives := 0
	const numRandomTests = 10_000_000

	for i := 0; i < numRandomTests; i++ {
		// Test if the random hash is in the bloom filter
		if bloom.Test(hash[:]) {
			falsePositives++
		}
		hash = sha256.Sum256(hash[:])
	}

	// Calculate actual false positive rate
	actualFalsePositiveRate := float64(falsePositives) / float64(numRandomTests) * 100
	estimatedFalsePositiveRate := bloom.EstimateFalsePositiveRate() * 100

	fmt.Printf("False positive test results:\n")
	fmt.Printf("  - Random hashes tested: %d\n", numRandomTests)
	fmt.Printf("  - False positives: %d\n", falsePositives)
	fmt.Printf("  - Actual false positive rate: %.8f%%\n", actualFalsePositiveRate)
	fmt.Printf("  - Estimated false positive rate: %.8f%%\n", estimatedFalsePositiveRate)
}
