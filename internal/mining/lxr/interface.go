// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxr

// SignatureVerification is an interface that abstracts the verification of mining signatures
// This helps break circular dependencies between packages
type SignatureVerification interface {
	// GetNonce returns the nonce used for mining
	GetNonce() []byte
	
	// GetComputedHash returns the computed hash from mining
	GetComputedHash() [32]byte
	
	// GetBlockHash returns the block hash used for mining
	GetBlockHash() [32]byte
}
