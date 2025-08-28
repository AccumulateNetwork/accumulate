// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/lxrpow"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Constants for LXR mining
const (
	// DefaultTableBits is the default size of memory table in bits (2^bits = size)
	// 20 bits = 1MB for testing, 30 bits = 1GB for production
	DefaultTableBits = 20
	// DefaultLoops is the number of loops through the hash
	DefaultLoops = 5
	// DefaultPasses is the default number of passes to randomize the byte map
	DefaultPasses = 6
	// MaxMiningAttempts is the maximum number of mining attempts
	MaxMiningAttempts = ^uint64(0) >> 1 // Half of uint64 max to avoid infinite loop
)

// LXR instance cache to avoid regenerating tables
var (
	lxrCache = make(map[uint64]*lxrpow.LxrPow)
	lxrMutex sync.RWMutex
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
	
	// Create and cache new instance
	lxr := lxrpow.NewLxrPow(loops, tableBits, passes)
	lxrCache[key] = lxr
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
func (s *LXRMiningSignature) VerifyMining(msg Signable) bool {
	// Extract nonce from WorkProof (stored at bytes 16-24)
	storedNonce := binary.BigEndian.Uint64(s.WorkProof[16:24])
	if storedNonce != s.Nonce {
		return false
	}
	
	// Get message hash
	msgHash := msg.Hash()
	
	// Verify first 8 bytes of message hash match what's in proof
	if !bytesEqual(msgHash[:8], s.WorkProof[8:16]) {
		return false
	}
	
	// Get LXR instance with same configuration
	tableSize := s.TableSize
	if tableSize == 0 {
		tableSize = DefaultTableBits
	}
	passes := s.Passes
	if passes == 0 {
		passes = DefaultPasses
	}
	lxr := getLXRInstance(uint64(tableSize), DefaultLoops, uint64(passes))
	
	// Recalculate the proof of work
	_, pow := lxr.LxrPoWHash(msgHash[:], s.Nonce)
	
	// Check if it meets the difficulty requirement
	return checkLXRDifficulty(pow, s.Difficulty)
}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Mine performs proof-of-work mining to find a valid nonce
func (s *LXRMiningSignature) Mine(msg Signable, targetDifficulty uint64) error {
	if s.PublicKey == nil {
		return errors.BadRequest.With("public key is required for mining")
	}
	
	s.Difficulty = targetDifficulty
	msgHash := msg.Hash()
	
	// Set default table configuration if not set
	if s.TableSize == 0 {
		s.TableSize = DefaultTableBits
	}
	if s.Passes == 0 {
		s.Passes = DefaultPasses
	}
	// TableSeed is no longer used with the real LXR algorithm
	
	// Get LXR instance
	lxr := getLXRInstance(uint64(s.TableSize), DefaultLoops, uint64(s.Passes))
	
	// Try different nonces until we find one that meets difficulty
	for nonce := uint64(0); nonce < MaxMiningAttempts; nonce++ {
		s.Nonce = nonce
		
		// Use LXR algorithm to compute proof of work
		_, pow := lxr.LxrPoWHash(msgHash[:], nonce)
		
		// Check if it meets difficulty using LXR's proof-of-work value
		if checkLXRDifficulty(pow, targetDifficulty) {
			// Store the proof as a hash (first 32 bytes of the pow value)
			var proof [32]byte
			binary.BigEndian.PutUint64(proof[:8], pow)
			// Include transaction and nonce info in proof for verification
			copy(proof[8:16], msgHash[:8])
			binary.BigEndian.PutUint64(proof[16:24], nonce)
			copy(proof[24:], s.PublicKey[:min(8, len(s.PublicKey))])
			s.WorkProof = proof
			return nil
		}
	}
	
	return errors.InternalError.With("failed to find valid nonce")
}

// checkLXRDifficulty checks if an LXR proof-of-work value meets the target difficulty
// The LXR PoW value encodes the difficulty as leading FF bytes followed by the hash value
func checkLXRDifficulty(pow uint64, targetDifficulty uint64) bool {
	// LXR proof-of-work uses a different difficulty metric
	// Higher pow value = more difficulty
	// We scale the difficulty to match LXR's proof-of-work values
	
	// Count leading FF bytes in the pow value
	leadingFFBytes := uint64(0)
	for i := uint(56); i > 0; i -= 8 {
		if byte(pow>>i) == 0xFF {
			leadingFFBytes++
		} else {
			break
		}
	}
	
	// Convert our difficulty scale to LXR's scale
	// Our difficulty 1000 = approximately 1 in 1000 chance
	// Map this to LXR's leading FF bytes requirement
	requiredLeadingBytes := targetDifficulty / 10000
	if requiredLeadingBytes > 8 {
		requiredLeadingBytes = 8
	}
	
	// Check if we have enough leading FF bytes
	if leadingFFBytes >= requiredLeadingBytes {
		return true
	}
	
	// For lower difficulties, check the remaining value
	if requiredLeadingBytes == 0 {
		// Use threshold check for low difficulties
		maxValue := ^uint64(0) / targetDifficulty
		return pow >= maxValue
	}
	
	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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