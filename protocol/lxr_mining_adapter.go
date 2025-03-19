// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

// GetNonce returns the nonce used for mining
func (s *LxrMiningSignature) GetNonce() []byte {
	return s.Nonce
}

// GetComputedHash returns the computed hash from mining
func (s *LxrMiningSignature) GetComputedHash() [32]byte {
	return s.ComputedHash
}

// GetBlockHash returns the block hash used for mining
func (s *LxrMiningSignature) GetBlockHash() [32]byte {
	return s.BlockHash
}
