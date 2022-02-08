package pmt

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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
func (b *BPT) CollectReceipt(BIdx, bit byte, node *Node, key [32]byte, receipt *managed.Receipt) (hash []byte) {

	step := func() { //  In order to reduce redundant code, we step with a
		bit <<= 1     // local function.         Inlining might provide some
		if bit == 0 { //                         performance.  What we are doing is shifting the
			bit = 1 //                           bit test up on each level of the Merkle tree.  If the bit
			BIdx++  //                           shifts out of a BIdx, we increment the BIdx and start over
		}
	}

	search := func(e *Entry) (hash []byte) { //                        Again, to avoid redundant code, Left and Right
		switch { //                                                    processing is done once here.
		case *e == nil: //                                             If we hit an empty entry in our sort, then the
			return nil //                                                    key is just not there.
		case (*e).T() == TNode: //                                     If the entry isn't nil, check if it is a Node
			step()                                                         // If it is a node, then try and insert it on that node
			return b.CollectReceipt(BIdx, bit, (*e).(*Node), key, receipt) //   Recurse up the tree
		default: //                                                    If not a node, not nil, it is a value.
			v := (*e).(*Value)                 //                      A collision. Get the value that got here first
			if bytes.Equal(key[:], v.Key[:]) { //                      If this value is the same as we are looking for
				receipt.Element = v.Hash[:] //                           Then this is the starting point for the receipt
				return append([]byte{}, v.Hash[:]...)
			} //
			return nil //                The idea is to create a node, to replace the value
		}
	}

	if node.Left != nil && node.Left.T() == TNotLoaded ||
		node.Right != nil && node.Right.T() == TNotLoaded {
		node.BBKey = GetBBKey(BIdx, key)
		n := b.manager.LoadNode(node)
		node.Left = n.Left
		node.Right = n.Right
	}

	if bit&key[BIdx] == 0 { //      Note that this is the code that calls the Inline function Insert, and Insert
		hash = search(&node.Left) //       in turn calls step.  We check the bit on the given BIdx. 0 goes Left
		if hash == nil {
			return nil
		}
		if &node.Right != nil {
			receipt.Nodes = append(receipt.Nodes, &managed.ReceiptNode{Hash: hash, Right: true})
		}
	} else { //
		hash = search(&node.Right) //
		if hash == nil {
			return nil
		}
		if &node.Right != nil {
			receipt.Nodes = append(receipt.Nodes, &managed.ReceiptNode{Hash: hash, Right: false})
		}
	}
	return hash
}

// GetReceipt
// Returns the receipt for the current state for the given chainID
func (b *BPT) GetReceipt(chainID [32]byte) *managed.Receipt { //          The location of a value is determined by the chainID (a key)
	if debug {
		fmt.Printf("BPT insert key=%v\n", storage.Key(chainID))
	}
	receipt := new(managed.Receipt)
	receipt.MDRoot = b.Root.Hash[:]
	b.CollectReceipt(0, 1, b.Root, chainID, receipt) //
	if receipt.MDRoot == nil {
		return nil
	}
	return receipt
} //
