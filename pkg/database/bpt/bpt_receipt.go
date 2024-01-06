// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// GetReceipt constructs a receipt for the current state for the given key.
func (b *BPT) GetReceipt(key *record.Key) (*merkle.Receipt, error) {
	const debug = false

	err := b.executePending()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Find the leaf node
	n, err := b.getRoot().getLeaf(key.Hash())
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Walk up the tree
	receipt := new(merkle.Receipt)
	receipt.Start = n.Hash[:]
	working := n.Hash
	var br *branch
	for n := node(n); ; n = br {
		// Get the parent
		switch n := n.(type) {
		case *branch:
			br = n.parent
		case *leaf:
			br = n.parent
		default:
			panic("invalid node")
		}
		if br == nil {
			break
		}

		// Skip any branches that only have one side populated
		nh, _ := n.getHash()
		brh, _ := br.getHash()
		if nh == brh {
			continue
		}

		// Calculate the next entry
		var entry *merkle.ReceiptEntry
		switch n {
		case br.Right:
			h, _ := br.Left.getHash()
			entry = &merkle.ReceiptEntry{Hash: h[:], Right: false}
			if debug {
				working = sha256.Sum256(append(h[:], working[:]...))
			}

		case br.Left:
			h, _ := br.Right.getHash()
			entry = &merkle.ReceiptEntry{Hash: h[:], Right: true}
			if debug {
				working = sha256.Sum256(append(working[:], h[:]...))
			}

		default:
			panic("invalid tree")
		}
		if debug && brh != working {
			panic("inconsistent BPT")
		}

		// Add it and move on
		receipt.Entries = append(receipt.Entries, entry)
	}

	h, _ := b.getRoot().getHash()
	receipt.Anchor = h[:]

	if !receipt.Validate(nil) {
		panic("constructed invalid receipt")
	}
	return receipt, nil
}
