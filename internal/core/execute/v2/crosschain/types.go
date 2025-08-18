// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// CollectionProof represents a collection proof for multiple transactions
type CollectionProof struct {
	Elements      [][]byte
	Receipt       *merkle.Receipt
	MessageCount  int
	MessageHashes [][32]byte
	StartSequence uint64
	EndSequence   uint64
}

// ConductorConfig contains configuration for the CrossChainConductor
type ConductorConfig struct {
	// ForceCollectionProofs ensures all transactions use collection proofs (no fallback)
	ForceCollectionProofs bool
	// CollectionMaxBatchSize is the maximum number of transactions per collection proof
	CollectionMaxBatchSize int
}