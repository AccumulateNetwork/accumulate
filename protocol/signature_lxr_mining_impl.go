// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package protocol implements the Accumulate protocol types and logic.
//
// LXRMiningSignature Implementation Note:
// This is a simplified proof-of-concept implementation that uses SHA256
// instead of the actual LXR hash algorithm. A production implementation
// would need to implement the full memory-hard LXR hash algorithm from
// the Factom PegNet specification to achieve the intended anti-spam
// protection through memory-hard proof-of-work.

package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Constants for LXR mining
const (
	// DefaultTableSize is the default size of memory table (power of 2)
	DefaultTableSize = 20 // 1MB for testing, should be 30 (1GB) for production
	// DefaultTableSeed is the default seed for table generation
	DefaultTableSeed = 0xDEADBEEF
	// DefaultPasses is the default number of passes through the table
	DefaultPasses = 5
	// ProgressCheckInterval is how often to check for progress/timeout
	ProgressCheckInterval = 1000000
	// MaxMiningAttempts is the maximum number of mining attempts
	MaxMiningAttempts = ^uint64(0) >> 1 // Half of uint64 max to avoid infinite loop
)

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
	// Calculate the mining hash input
	msgHash := msg.Hash()
	input := make([]byte, 32+8+len(s.PublicKey))
	copy(input, msgHash[:])
	binary.BigEndian.PutUint64(input[32:], s.Nonce)
	copy(input[40:], s.PublicKey)
	
	// For this simplified implementation, we just use SHA256
	// A real implementation would use the LXRHash algorithm
	proof := sha256.Sum256(input)
	
	// Check if the proof matches
	if proof != s.WorkProof {
		return false
	}
	
	// Check if the proof meets the difficulty requirement
	return checkDifficulty(proof[:], s.Difficulty)
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
		s.TableSize = DefaultTableSize
	}
	if s.TableSeed == 0 {
		s.TableSeed = DefaultTableSeed
	}
	if s.Passes == 0 {
		s.Passes = DefaultPasses
	}
	
	// Try different nonces until we find one that meets difficulty
	for nonce := uint64(0); nonce < MaxMiningAttempts; nonce++ {
		s.Nonce = nonce
		
		// Calculate the mining hash
		input := make([]byte, 32+8+len(s.PublicKey))
		copy(input, msgHash[:])
		binary.BigEndian.PutUint64(input[32:], nonce)
		copy(input[40:], s.PublicKey)
		
		// For this simplified implementation, we just use SHA256
		// A real implementation would use the LXRHash algorithm
		proof := sha256.Sum256(input)
		
		// Check if it meets difficulty
		if checkDifficulty(proof[:], targetDifficulty) {
			s.WorkProof = proof
			return nil
		}
	}
	
	return errors.InternalError.With("failed to find valid nonce")
}

// checkDifficulty checks if a hash meets the target difficulty
// Difficulty is measured as the number of leading zero bits required
func checkDifficulty(hash []byte, difficulty uint64) bool {
	// Simple difficulty check: count leading zeros
	// difficulty = 1000 means approximately 1 in 1000 chance
	// We use a simple threshold check for simplicity
	
	// Convert first 8 bytes to uint64
	if len(hash) < 8 {
		return false
	}
	
	hashValue := binary.BigEndian.Uint64(hash[:8])
	
	// Higher difficulty = lower maximum hash value
	// Max value decreases exponentially with difficulty
	maxValue := ^uint64(0) / difficulty
	
	return hashValue <= maxValue
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