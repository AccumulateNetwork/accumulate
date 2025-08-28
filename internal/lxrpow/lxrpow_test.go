// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxrpow

import (
	"crypto/sha256"
	"testing"
)

func TestLxrPow_Init(t *testing.T) {
	lx := NewLxrPow(5, 20, 3) // Use smaller values for testing
	
	if lx.Loops != 5 {
		t.Errorf("Expected Loops to be 5, got %d", lx.Loops)
	}
	
	if lx.MapBits != 20 {
		t.Errorf("Expected MapBits to be 20, got %d", lx.MapBits)
	}
	
	if lx.MapSize != 1<<20 {
		t.Errorf("Expected MapSize to be %d, got %d", 1<<20, lx.MapSize)
	}
	
	if lx.Passes != 3 {
		t.Errorf("Expected Passes to be 3, got %d", lx.Passes)
	}
	
	if len(lx.ByteMap) != 1<<20 {
		t.Errorf("Expected ByteMap length to be %d, got %d", 1<<20, len(lx.ByteMap))
	}
}

func TestLxrPow_LxrPoW(t *testing.T) {
	// Create a smaller LxrPow instance for testing
	lx := NewLxrPow(3, 20, 2)
	
	// Create a test hash
	hash := sha256.Sum256([]byte("test"))
	
	// Test with different nonces
	pow1 := lx.LxrPoW(hash[:], 1)
	pow2 := lx.LxrPoW(hash[:], 2)
	
	// Verify that different nonces produce different PoW values
	if pow1 == pow2 {
		t.Errorf("Expected different PoW values for different nonces, got %d for both", pow1)
	}
	
	// Test with the same nonce to verify determinism
	pow3 := lx.LxrPoW(hash[:], 1)
	if pow1 != pow3 {
		t.Errorf("Expected same PoW value for same nonce, got %d and %d", pow1, pow3)
	}
}

func TestLxrPow_LxrPoWHash(t *testing.T) {
	// Create a smaller LxrPow instance for testing
	lx := NewLxrPow(3, 20, 2)
	
	// Create a test hash
	hash := sha256.Sum256([]byte("test"))
	
	// Test with a nonce
	newHash, pow := lx.LxrPoWHash(hash[:], 1)
	
	// Verify that the hash is not the same as the input
	if len(newHash) != 32 {
		t.Errorf("Expected hash length to be 32, got %d", len(newHash))
	}
	
	// Verify that the PoW value is the same as returned by LxrPoW
	pow2 := lx.LxrPoW(hash[:], 1)
	if pow != pow2 {
		t.Errorf("Expected same PoW value from LxrPoWHash and LxrPoW, got %d and %d", pow, pow2)
	}
}

func TestLxrPow_Avalanche(t *testing.T) {
	// Create a smaller LxrPow instance for testing
	lx := NewLxrPow(3, 20, 2)
	
	// Create a test hash
	hash1 := sha256.Sum256([]byte("test"))
	
	// Create a slightly different hash (change one bit)
	hash2 := hash1
	hash2[0] ^= 1 // Flip the lowest bit of the first byte
	
	// Test with the same nonce
	pow1 := lx.LxrPoW(hash1[:], 1)
	pow2 := lx.LxrPoW(hash2[:], 1)
	
	// Verify that different hashes produce different PoW values
	if pow1 == pow2 {
		t.Errorf("Expected different PoW values for different hashes, got %d for both", pow1)
	}
}

func BenchmarkLxrPow(b *testing.B) {
	// Create a smaller LxrPow instance for benchmarking
	lx := NewLxrPow(3, 20, 2)
	
	// Create a test hash
	hash := sha256.Sum256([]byte("test"))
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lx.LxrPoW(hash[:], uint64(i))
	}
}

func TestDefaultLxrPow(t *testing.T) {
	lx := DefaultLxrPow()
	
	if lx.Loops != 5 {
		t.Errorf("Expected Loops to be 5, got %d", lx.Loops)
	}
	
	if lx.MapBits != 30 {
		t.Errorf("Expected MapBits to be 30, got %d", lx.MapBits)
	}
	
	if lx.MapSize != 1<<30 {
		t.Errorf("Expected MapSize to be %d, got %d", 1<<30, lx.MapSize)
	}
	
	if lx.Passes != 6 {
		t.Errorf("Expected Passes to be 6, got %d", lx.Passes)
	}
}
