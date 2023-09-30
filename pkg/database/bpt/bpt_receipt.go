// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

// collectReceipt
// A recursive routine that searches the BPT for the given chainID.  Once it is
// found, the search unwinds and builds the receipt.
//
// Inputs:
// BIdx -- byte index into the key
// bit  -- index to the bit
// node -- the node in the BPT where we have reached in our search so far
// key  -- The key in the BPT we are looking for
func (b *BPT) collectReceipt(BIdx, bit byte, n *branch, key [32]byte, r *merkle.Receipt) (hash []byte) {
	// Load the node and hope it doesn't fail
	_ = n.load()

	var entry, other node  // The node has a left or right entry that builds a tree.
	bite := key[BIdx]      // Get the byte for debugging.
	right := bit&bite == 0 // Flag for going right or left up the tree depends on a bit in the key
	entry = n.Right        // Guess we are going right (that the current bit is 1)
	other = n.Left         // We will need the other path as well.
	if !right {            // If the bit isn't 1, then we are NOT going right
		entry = n.Left  //         then go left
		other = n.Right //        and the right will be the other path
	}

	value, ok := entry.(*leaf)
	if ok {
		if value.Key.Hash() == key {
			r.Start = append(r.Start[:0], value.Hash[:]...)
			if other != nil { // If other isn't nil, then add it to the node list of the receipt
				h, _ := other.getHash()
				r.Entries = append(r.Entries,
					&merkle.ReceiptEntry{Hash: h[:], Right: !right})
			}
			return append([]byte{}, n.Hash[:]...) // Note that the node.Hash is combined with other if other != nil
		}
		return nil
	}
	nextNode, ok := entry.(*branch)
	if !ok {
		return nil
	}

	// We have processed the current bit.  Now move to the next bit.
	// Increment the bit index. If the set bit is still in the byte, we are done.
	// If the bit rolls out of the byte, then set the low order bit, and increment the byte index.
	bit >>= 1     //
	if bit == 0 { //   performance.  What we are doing is shifting the
		bit = 0x80 //  bit test up on each level of the Merkle tree.  If the bit
		BIdx++     //  shifts out of a BIdx, we increment the BIdx and start over
	}

	childhash := b.collectReceipt(BIdx, bit, nextNode, key, r)
	if childhash == nil {
		return nil
	}

	if other != nil {
		// Add the hash to the receipt provided by the entry, and mark it right or not right (right flag)
		h, _ := other.getHash()
		r.Entries = append(r.Entries, &merkle.ReceiptEntry{Hash: h[:], Right: !right})
	}

	h, _ := n.getHash()
	return h[:]
}

// GetReceipt
// Returns the receipt for the current state for the given chainID
func (b *BPT) GetReceipt(key [32]byte) (*merkle.Receipt, error) { //          The location of a value is determined by the chainID (a key)
	err := b.executePending()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	receipt := new(merkle.Receipt)
	receipt.Anchor = b.collectReceipt(0, 0x80, b.getRoot(), key, receipt) //
	if receipt.Anchor == nil {
		return nil, errors.NotFound.WithFormat("key %x not found", key)
	}
	return receipt, nil
} //
