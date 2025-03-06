// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// RoutingLocation returns the URL of the signer
func (s *LxrMiningSignature) RoutingLocation() *url.URL {
	return s.Signer
}

// GetVote returns how the signer votes on a particular transaction
func (s *LxrMiningSignature) GetVote() VoteType {
	return VoteTypeAccept // Default to accept
}

// GetSigner returns the URL of the signer
func (s *LxrMiningSignature) GetSigner() *url.URL {
	return s.Signer
}

// GetTransactionHash returns the hash of the transaction
func (s *LxrMiningSignature) GetTransactionHash() [32]byte {
	var hash [32]byte
	// If TransactionHash is not set, we'll return an empty hash
	// This will be populated by the transaction processor
	return hash
}

// Hash returns the hash of the signature
func (s *LxrMiningSignature) Hash() []byte {
	h := sha256.New()
	h.Write(s.Nonce)
	h.Write(s.ComputedHash[:])
	h.Write(s.BlockHash[:])
	h.Write([]byte(s.Signer.String()))
	h.Write([]byte{byte(s.SignerVersion >> 56), byte(s.SignerVersion >> 48), byte(s.SignerVersion >> 40), byte(s.SignerVersion >> 32), byte(s.SignerVersion >> 24), byte(s.SignerVersion >> 16), byte(s.SignerVersion >> 8), byte(s.SignerVersion)})
	h.Write([]byte{byte(s.Timestamp >> 56), byte(s.Timestamp >> 48), byte(s.Timestamp >> 40), byte(s.Timestamp >> 32), byte(s.Timestamp >> 24), byte(s.Timestamp >> 16), byte(s.Timestamp >> 8), byte(s.Timestamp)})
	return h.Sum(nil)
}

// Metadata returns the signature's metadata
func (s *LxrMiningSignature) Metadata() Signature {
	return &LxrMiningSignature{
		Signer:        s.Signer,
		SignerVersion: s.SignerVersion,
		Timestamp:     s.Timestamp,
	}
}

// UnionValue returns the signature as a union value
func (s *LxrMiningSignature) UnionValue() interface{} {
	return s
}

// KeyPageOperationSetMiningParameters defines an operation to set mining parameters for a key page
type KeyPageOperationSetMiningParameters struct {
	fieldsSet []bool
	// Enable or disable mining for the key page
	Enabled bool `json:"enabled,omitempty" form:"enabled" query:"enabled"`
	// Minimum difficulty required for mining submissions
	Difficulty uint64 `json:"difficulty,omitempty" form:"difficulty" query:"difficulty"`
	extraData  []byte
}

// Type returns the operation type
func (o *KeyPageOperationSetMiningParameters) Type() KeyPageOperationType {
	return KeyPageOperationTypeSetMiningParametersOperation
}

// UnionValue returns the operation as a union value
func (o *KeyPageOperationSetMiningParameters) UnionValue() interface{} {
	return o
}
