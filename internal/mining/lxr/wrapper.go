// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxr

import (
	"crypto/sha256"
	"encoding/binary"
	
	"gitlab.com/accumulatenetwork/accumulate/internal/lxrpow"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Hasher provides a wrapper around the LXRPoW implementation
type Hasher struct {
	lxr *lxrpow.LxrPow
}

// IsTestEnvironment is a flag to indicate whether we're running in a test environment
// This is used to create a smaller hash table for tests
var IsTestEnvironment bool

// NewHasher creates a new LXR hasher with default settings
func NewHasher() *Hasher {
	if IsTestEnvironment {
		// Use very small settings for tests
		// 1 loop, 12 bits (4KB byte map), 1 pass
		lxr := lxrpow.NewLxrPow(1, 12, 1)
		return &Hasher{lxr: lxr}
	}
	
	// Use the default LXRPoW settings for production
	// 5 loops, 30 bits (1GB byte map), 6 passes
	lxr := lxrpow.NewLxrPow(5, 30, 6)
	return &Hasher{lxr: lxr}
}

// NewCustomHasher creates a new LXR hasher with custom settings
func NewCustomHasher(loops, bits, passes uint64) *Hasher {
	lxr := lxrpow.NewLxrPow(loops, bits, passes)
	return &Hasher{lxr: lxr}
}

// CalculatePow calculates the proof of work for a given block hash and nonce
// Returns the computed hash and the difficulty value
func (h *Hasher) CalculatePow(blockHash []byte, nonce []byte) ([32]byte, uint64) {
	// Ensure we have a 32-byte hash
	var hashData [32]byte
	if len(blockHash) == 32 {
		copy(hashData[:], blockHash)
	} else {
		// If the block hash is not 32 bytes, hash it to get a 32-byte hash
		hashData = sha256.Sum256(append(blockHash, nonce...))
	}
	
	// Convert nonce to uint64 for LxrPoWHash
	var nonceVal uint64
	if len(nonce) >= 8 {
		nonceVal = binary.LittleEndian.Uint64(nonce)
	}
	
	hash, difficulty := h.lxr.LxrPoWHash(hashData[:], nonceVal)
	
	// Convert the hash to a fixed-size array
	var result [32]byte
	copy(result[:], hash)
	
	return result, difficulty
}

// VerifySignature verifies an LxrMiningSignature
// Returns true if the signature is valid, false otherwise
func (h *Hasher) VerifySignature(sig *protocol.LxrMiningSignature, minDifficulty uint64) bool {
	// Check if the signature is nil
	if sig == nil {
		return false
	}
	
	// Calculate the proof of work
	computedHash, difficulty := h.CalculatePow(sig.BlockHash[:], sig.Nonce)
	
	// Check if the difficulty meets the minimum requirement
	if difficulty < minDifficulty {
		return false
	}
	
	// Check if the computed hash matches the one in the signature
	for i := 0; i < 32; i++ {
		if computedHash[i] != sig.ComputedHash[i] {
			return false
		}
	}
	
	return true
}

// CheckDifficulty checks if a hash meets a minimum difficulty requirement
func (h *Hasher) CheckDifficulty(hash []byte, nonce []byte, minDifficulty uint64) (bool, uint64) {
	_, difficulty := h.CalculatePow(hash, nonce)
	return difficulty >= minDifficulty, difficulty
}
