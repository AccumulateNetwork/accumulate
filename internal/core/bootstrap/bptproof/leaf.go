// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bptproof returns BPT leaves with Merkle proofs against the
// current BPT root. Backs the GetBptLeaf service method (issue #3958)
// for the minimum-data node bootstrap (issue #3953).
package bptproof

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// LeafWithProof is a BPT leaf plus a Merkle proof rooted at the current BPT
// root.
type LeafWithProof struct {
	KeyHash   [32]byte
	ValueHash [32]byte
	Proof     *merkle.Receipt
	BptRoot   [32]byte
}

// GetLeaf returns the leaf at keyHash (if any) plus a proof to the current
// BPT root.
func GetLeaf(batch *database.Batch, keyHash [32]byte) (*LeafWithProof, error) {
	rootHash, err := batch.GetBptRootHash()
	if err != nil {
		return nil, fmt.Errorf("get bpt root: %w", err)
	}

	key := record.KeyFromHash(keyHash)
	receipt, err := batch.BPT().GetReceipt(key)
	if err != nil {
		return nil, fmt.Errorf("get receipt: %w", err)
	}
	if receipt == nil {
		return nil, fmt.Errorf("no leaf at key %x", keyHash[:])
	}

	var value [32]byte
	if len(receipt.Start) == 32 {
		copy(value[:], receipt.Start)
	}
	var anchor [32]byte
	if len(receipt.Anchor) == 32 {
		copy(anchor[:], receipt.Anchor)
	}

	if anchor != rootHash {
		return nil, fmt.Errorf("inconsistent receipt anchor (proof=%x, root=%x)", anchor[:8], rootHash[:8])
	}

	return &LeafWithProof{
		KeyHash:   keyHash,
		ValueHash: value,
		Proof:     receipt,
		BptRoot:   rootHash,
	}, nil
}
