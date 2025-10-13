// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/lxrpow"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Architecture Note:
// The LXR mining configuration (TableSize, Passes, etc.) is stored in the MiningAuthority
// account, not in the signature itself. The signature only contains the proof results
// (Nonce, Difficulty, WorkProof). Verification should be performed at the executor level
// where the MiningAuthority can be fetched from the database to get the configuration.
//
// The methods here use default values for basic validation, but proper validation
// should fetch the MiningAuthority and use its configuration parameters.
//
// Replay Protection:
// The mining proof incorporates multiple elements to prevent replay attacks:
// 1. Timestamp - Must be set and is XORed into the mining input
// 2. SignerVersion - Changes when keys are updated, invalidating old proofs
// 3. Message Hash - Ties the proof to a specific transaction
// 4. Public Key Hash - Ensures proof is tied to specific signer
//
// The combination of timestamp + signer version makes each proof unique even for
// identical transactions, preventing replay attacks while maintaining the ability
// to verify the proof later.

// Constants for LXR mining
const (
	// DefaultTableBits is the default size of memory table in bits (2^bits = size)
	// TODO: Change to 30 (1GB) for production deployment
	// 20 bits = 1MB for testing, 30 bits = 1GB for production
	DefaultTableBits = 20
	// DefaultLoops is the number of loops through the hash
	DefaultLoops = 5
	// DefaultPasses is the default number of passes to randomize the byte map
	DefaultPasses = 6
	// MaxMiningAttempts is the maximum number of mining attempts
	MaxMiningAttempts = ^uint64(0) >> 1 // Half of uint64 max to avoid infinite loop
	// MaxCacheEntries limits the number of cached LXR instances
	MaxCacheEntries = 10

	// WorkProof encoding offsets
	WorkProofPowOffset    = 0  // Bytes 0-8: LXR PoW value
	WorkProofHashOffset   = 8  // Bytes 8-16: Message hash prefix
	WorkProofNonceOffset  = 16 // Bytes 16-24: Mining nonce
	WorkProofPubKeyOffset = 24 // Bytes 24-32: Public key prefix
)

// LXR instance cache to avoid regenerating tables
var (
	lxrCache      = make(map[uint64]*lxrpow.LxrPow)
	lxrCacheOrder []uint64 // Track insertion order for LRU eviction
	lxrMutex      sync.RWMutex
)

// getLXRInstance returns a cached LXR instance or creates a new one
func getLXRInstance(tableBits, loops, passes uint64) *lxrpow.LxrPow {
	// Create a unique key for the configuration
	key := (tableBits << 32) | (loops << 16) | passes

	// Check cache with read lock
	lxrMutex.RLock()
	if lxr, ok := lxrCache[key]; ok {
		lxrMutex.RUnlock()
		return lxr
	}
	lxrMutex.RUnlock()

	// Create new instance with write lock
	lxrMutex.Lock()
	defer lxrMutex.Unlock()

	// Double-check in case another goroutine created it
	if lxr, ok := lxrCache[key]; ok {
		return lxr
	}

	// Evict oldest entry if cache is full (LRU)
	if len(lxrCache) >= MaxCacheEntries {
		if len(lxrCacheOrder) > 0 {
			oldestKey := lxrCacheOrder[0]
			delete(lxrCache, oldestKey)
			lxrCacheOrder = lxrCacheOrder[1:]
		}
	}

	// Create and cache new instance
	lxr := lxrpow.NewLxrPow(loops, tableBits, passes)
	lxrCache[key] = lxr
	lxrCacheOrder = append(lxrCacheOrder, key)
	return lxr
}

// RoutingLocation returns the signer URL
func (s *LXRMiningSignature) RoutingLocation() *url.URL {
	return s.Signer
}

// GetVote returns the vote type
func (s *LXRMiningSignature) GetVote() VoteType {
	return s.Vote
}

// GetSigner returns the signer URL
func (s *LXRMiningSignature) GetSigner() *url.URL {
	return s.Signer
}

// GetTransactionHash returns the transaction hash
func (s *LXRMiningSignature) GetTransactionHash() [32]byte {
	return s.TransactionHash
}

// Hash returns the hash of the signature
func (s *LXRMiningSignature) Hash() []byte {
	return signatureHash(s)
}

// Metadata returns a copy with signature data removed
func (s *LXRMiningSignature) Metadata() Signature {
	cpy := *s
	cpy.Signature = nil
	cpy.TransactionHash = [32]byte{}
	return &cpy
}

// Initiator creates the deprecated initiator hash for the signature
func (s *LXRMiningSignature) Initiator() (hash.Hasher, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	// Create the initiator hash
	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher, nil
}

// GetSignature returns the signature bytes
func (s *LXRMiningSignature) GetSignature() []byte {
	return s.Signature
}

// GetPublicKeyHash returns the hash of the public key
func (s *LXRMiningSignature) GetPublicKeyHash() []byte {
	if s.PublicKey == nil {
		return nil
	}
	hash := sha256.Sum256(s.PublicKey)
	return hash[:]
}

// GetPublicKey returns the public key
func (s *LXRMiningSignature) GetPublicKey() []byte {
	return s.PublicKey
}

// GetSignerVersion returns the signer version
func (s *LXRMiningSignature) GetSignerVersion() uint64 {
	return s.SignerVersion
}

// GetTimestamp returns the timestamp
func (s *LXRMiningSignature) GetTimestamp() uint64 {
	return s.Timestamp
}

// Verify verifies that the signature is valid for the given message
func (s *LXRMiningSignature) Verify(sig Signature, msg Signable) bool {
	// Cast to get the actual signature
	lxrSig, ok := sig.(*LXRMiningSignature)
	if !ok {
		return false
	}

	// First verify the mining proof
	if !s.VerifyMining(msg) {
		return false
	}

	// Then verify the cryptographic signature
	// The signature should be over the work proof
	if len(lxrSig.PublicKey) != ed25519.PublicKeySize {
		return false
	}

	if len(lxrSig.Signature) != ed25519.SignatureSize {
		return false
	}

	// Verify signature of the work proof
	return ed25519.Verify(lxrSig.PublicKey, lxrSig.WorkProof[:], lxrSig.Signature)
}

// VerifyMining verifies that the proof-of-work is valid for the given message
// WorkProof layout: [0:8] = PoW value, [8:16] = msg hash prefix,
// [16:24] = nonce, [24:32] = public key prefix
func (s *LXRMiningSignature) VerifyMining(msg Signable) bool {
	// Extract nonce from WorkProof
	storedNonce := binary.BigEndian.Uint64(s.WorkProof[WorkProofNonceOffset : WorkProofNonceOffset+8])
	if storedNonce != s.Nonce {
		return false
	}

	// Get message hash
	msgHash := msg.Hash()

	// Verify first 8 bytes of message hash match what's in proof (constant-time)
	if subtle.ConstantTimeCompare(msgHash[:8], s.WorkProof[WorkProofHashOffset:WorkProofHashOffset+8]) != 1 {
		return false
	}

	// Recreate the same mining input used during mining (includes replay protection)
	miningInput := make([]byte, 32)
	copy(miningInput, msgHash[:])
	// XOR in timestamp and signer version for uniqueness
	binary.BigEndian.PutUint64(miningInput[0:8], binary.BigEndian.Uint64(miningInput[0:8])^s.Timestamp)
	binary.BigEndian.PutUint64(miningInput[8:16], binary.BigEndian.Uint64(miningInput[8:16])^s.SignerVersion)

	// Get LXR instance with default configuration
	// The actual configuration should be validated at a higher level
	// where the MiningAuthority can be fetched from the database
	lxr := getLXRInstance(DefaultTableBits, DefaultLoops, DefaultPasses)

	// Recalculate the proof of work with the unique mining input
	_, pow := lxr.LxrPoWHash(miningInput, s.Nonce)

	// Check if it meets the difficulty requirement
	return checkLXRDifficulty(pow, s.Difficulty)
}

// Mine performs proof-of-work mining to find a valid nonce
func (s *LXRMiningSignature) Mine(msg Signable, targetDifficulty uint64) error {
	if s.PublicKey == nil {
		return errors.BadRequest.With("public key is required for mining")
	}

	// Ensure timestamp is set for replay protection
	if s.Timestamp == 0 {
		return errors.BadRequest.With("timestamp is required for mining")
	}

	s.Difficulty = targetDifficulty

	// Create mining input that includes message hash, timestamp, and signer version
	// for replay protection. This ensures the proof is unique to this specific
	// signature attempt.
	msgHash := msg.Hash()
	miningInput := make([]byte, 32)
	copy(miningInput, msgHash[:])
	// XOR in timestamp and signer version for uniqueness
	binary.BigEndian.PutUint64(miningInput[0:8], binary.BigEndian.Uint64(miningInput[0:8])^s.Timestamp)
	binary.BigEndian.PutUint64(miningInput[8:16], binary.BigEndian.Uint64(miningInput[8:16])^s.SignerVersion)

	// Get LXR instance with specified configuration
	// These would typically come from the MiningAuthority
	tableSize := uint64(DefaultTableBits)
	passes := uint64(DefaultPasses)
	lxr := getLXRInstance(tableSize, DefaultLoops, passes)

	// Try different nonces until we find one that meets difficulty
	for nonce := uint64(0); nonce < MaxMiningAttempts; nonce++ {
		s.Nonce = nonce

		// Use LXR algorithm to compute proof of work with the unique mining input
		_, pow := lxr.LxrPoWHash(miningInput, nonce)

		// Check if it meets difficulty using LXR's proof-of-work value
		if checkLXRDifficulty(pow, targetDifficulty) {
			// Store the proof with structured layout:
			// [0:8] = PoW value, [8:16] = msg hash prefix (original, not mining input),
			// [16:24] = nonce, [24:32] = public key prefix
			var proof [32]byte
			binary.BigEndian.PutUint64(proof[WorkProofPowOffset:], pow)
			copy(proof[WorkProofHashOffset:], msgHash[:8]) // Store original message hash
			binary.BigEndian.PutUint64(proof[WorkProofNonceOffset:], nonce)
			// Use first 8 bytes of public key hash for better uniqueness
			pubKeyHash := sha256.Sum256(s.PublicKey)
			copy(proof[WorkProofPubKeyOffset:], pubKeyHash[:8])
			s.WorkProof = proof
			return nil
		}
	}

	return errors.InternalError.WithFormat("failed to find valid nonce after %d attempts for difficulty %d", MaxMiningAttempts, targetDifficulty)
}

// checkLXRDifficulty checks if an LXR proof-of-work value meets the target difficulty.
//
// Difficulty Scale:
// The difficulty value represents the expected number of hashes needed to find a valid proof.
// - Difficulty 1 = ~1 hash (always passes)
// - Difficulty 256 = ~256 hashes (1 in 256 chance)
// - Difficulty 65536 = ~65,536 hashes (1 in 65,536 chance)
// - Difficulty 16777216 = ~16.7M hashes (requires 3 leading 0xFF bytes)
//
// The LXR PoW value encodes difficulty as leading 0xFF bytes:
// - 0 leading 0xFF bytes: pow < 0x00FFFFFFFFFFFFFF (common, low difficulty)
// - 1 leading 0xFF byte:  pow >= 0xFF00000000000000 (1 in 256 chance)
// - 2 leading 0xFF bytes: pow >= 0xFFFF000000000000 (1 in 65,536 chance)
// - 3 leading 0xFF bytes: pow >= 0xFFFFFF0000000000 (1 in 16.7M chance)
// - 8 leading 0xFF bytes: pow == 0xFFFFFFFFFFFFFFFF (practically impossible)
func checkLXRDifficulty(pow uint64, targetDifficulty uint64) bool {
	// Count leading 0xFF bytes in the PoW value
	leadingFFBytes := 0
	for shift := uint(56); shift <= 56; shift -= 8 {
		if byte(pow>>shift) == 0xFF {
			leadingFFBytes++
		} else {
			break
		}
	}

	// Calculate required leading 0xFF bytes based on difficulty
	// Each leading 0xFF byte represents 256x increase in difficulty
	// difficulty = 256^leadingBytes * remainingDifficulty
	requiredLeadingBytes := 0
	remainingDifficulty := targetDifficulty

	// Calculate how many full 0xFF bytes we need
	for remainingDifficulty >= 256 && requiredLeadingBytes < 8 {
		remainingDifficulty /= 256
		requiredLeadingBytes++
	}

	// Check if we have enough leading 0xFF bytes
	if leadingFFBytes > requiredLeadingBytes {
		return true
	}
	if leadingFFBytes < requiredLeadingBytes {
		return false
	}

	// If we have exactly the required leading bytes, check the remaining value
	if requiredLeadingBytes == 0 {
		// No leading 0xFF bytes required, use simple threshold
		threshold := ^uint64(0) / targetDifficulty
		return pow >= threshold
	}

	// Extract the non-0xFF portion and check against remaining difficulty
	mask := ^uint64(0) >> (uint(requiredLeadingBytes) * 8)
	nonFFPortion := pow & mask
	threshold := mask / remainingDifficulty

	return nonFFPortion >= threshold
}

// SignLXRMining signs the work proof with the private key
func SignLXRMining(sig *LXRMiningSignature, privKey ed25519.PrivateKey) error {
	if len(privKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key size")
	}

	// Sign the work proof
	sig.Signature = ed25519.Sign(privKey, sig.WorkProof[:])
	return nil
}
