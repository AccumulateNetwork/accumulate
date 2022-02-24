package pmt

import (
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

// CollectReceipt
// A recursive routine that searches the BPT for the given chainID.  Once it is
// found, the search unwinds and builds the receipt.
//
// Inputs:
// BIdx -- byte index into the key
// bit  -- index to the bit
// node -- the node in the BPT where we have reached in our search so far
// key  -- The key in the BPT we are looking for
func (b *BPT) CollectReceipt(BIdx, bit byte, node *BptNode, key [32]byte, receipt *managed.Receipt) (hash []byte) {
	if node.Left != nil && node.Left.T() == TNotLoaded || // Check if either the Left or Right
		node.Right != nil && node.Right.T() == TNotLoaded { // are pointing to not loaded.
		node.BBKey = GetBBKey(BIdx, key) // If not loaded, load the nodes
		n := b.manager.LoadNode(node)
		node.Left = n.Left
		node.Right = n.Right
	}

	var entry, other Entry // The node has a left or right entry that builds a tree.
	bite := key[BIdx]      // Get the byte for debugging.
	right := bit&bite == 0 // Flag for going right or left up the tree depends on a bit in the key
	entry = node.Right     // Guess we are going right (that the current bit is 1)
	other = node.Left      // We will need the other path as well.
	if !right {            // If the bit isn't 1, then we are NOT going right
		entry = node.Left  //         then go left
		other = node.Right //        and the right will be the other path
	}

	value, ok := entry.(*Value)
	if ok {
		if value.Key == key {
			receipt.Element = append(receipt.Element[:0], value.Hash[:]...)
			if other != nil { // If other isn't nil, then add it to the node list of the receipt
				receipt.Nodes = append(receipt.Nodes,
					&managed.ReceiptNode{Hash: other.GetHash(), Right: !right})
			}
			return append([]byte{}, node.Hash[:]...) // Note that the node.Hash is combined with other if other != nil
		}
		return nil
	}
	nextNode, ok := entry.(*BptNode)
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

	childhash := b.CollectReceipt(BIdx, bit, nextNode, key, receipt)
	if childhash == nil {
		return nil
	}

	if other != nil {
		// Add the hash to the receipt provided by the entry, and mark it right or not right (right flag)
		receipt.Nodes = append(receipt.Nodes, &managed.ReceiptNode{Hash: other.GetHash(), Right: !right})
	}

	return node.GetHash()
}

// GetReceipt
// Returns the receipt for the current state for the given chainID
func (b *BPT) GetReceipt(chainID [32]byte) *managed.Receipt { //          The location of a value is determined by the chainID (a key)
	receipt := new(managed.Receipt)
	receipt.MDRoot = b.CollectReceipt(0, 0x80, b.Root, chainID, receipt) //
	if receipt.MDRoot == nil {
		return nil
	}
	return receipt
} //
