// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

// Note that the tree considered here grows up by convention here, where
// Parent nodes are at the bottom, and leaves are at the top. Obviously
// mapping up to down and down to up is valid if a need to have the tree
// grow down is viewed as important.

// BptNode
// A node in our binary patricia/merkle tree
//
// A note about Byte Blocks.  Byte Blocks hold nodes and values, and are
// the structures serialized to persist nodes and values to the
// database, and load them as needed.  At least eventually that is what is
// to be done.
//
// The BBKey is an interesting thing in the context of Byte Blocks.  It is
// the key used to persist a byte block.  It is the bytes of the keys used
// by the BPT to track state.  Because Go doesn't allow slices as keys, and
// other solutions to make keys (like converting byte slices to strings)
// creates heavy load on the garbage collectors, we need a different
// solution.
//
// What we do is clear out all the bytes of the key other than the leading
// bytes of the key, those bits that were used to identify a particular
// byte block.  But of course, a key can have zero bytes.
//
// We give up 8 bits of the space and put the length of the key in the last
// byte.  This will only cause us issues if we use 248 bits of the key
// to create a node.  In other words, not too much of an issue.

type BptNode struct {
	Height  int      // Root is 0. above root is 1. Above above root is 2, etc.
	NodeKey [32]byte // Byte Block Key.
	Hash    [32]byte // This is the summary hash for the tree
	Left    Entry    // The hash of the child Left and up the tree, bit is zero
	Right   Entry    // the hash to the child Right and up the tree, bit is one
	Parent  *BptNode // the Parent node "below" this node
}

func (n *BptNode) Equal(entry Entry) (equal bool) {

	defer func() { //                          When an entry is cast to *Node, it might panic. Then the test is false
		if err := recover(); err != nil { //   Also left or right (which is not nil) may be compared to a nil.
			equal = false //                   That will also panic, and if so, false is returned.
		}
	}()

	// We compare only down the BPT.  If we compare both up and down the tree,
	// then the code would loop infinitely.  Certainly we could avoid retracing
	// paths, but if we wish to compare entire BPT trees, we can compare their
	// roots.
	node := entry.(*BptNode) //                           The entry considered must be a node.  If it is not
	switch {                 //                              this conversion will panic, get caught, and return false
	case n.Height != node.Height: //                      Height == Height
		return false //
	case n.NodeKey != node.NodeKey: //                    NodeKey == NodeKey
		return false //
	case n.Hash != node.Hash: //                          Hash == Hash
		return false //
	case n.Left == nil && node.Left != nil: //            Left both nil -- Later test of equality will throw a panic
		return false //                                                    which gets caught and returns false
	case n.Right == nil && node.Right != nil: //          Right both nil -- Later test of equality will throw a panic
		return false //                                                     which gets caught and returns false
	case n.Left != nil && !n.Left.Equal(node.Left): //    Left == Left
		return false //
	case n.Right != nil && !n.Right.Equal(node.Right): // Right == Right
		return false //
	} //
	return true //                                        All good! Equal
}

// T
// Returns true if this node is really a Node, and false if the node
// is really a Value.  We possibly want a way for Node to compress the
// distance to child nodes that have a long single path to leaves just
// because a key matches a number of bits before differentiating.
func (n *BptNode) T() int {
	return TNode
}

// GetHash
// Returns the Hash value for computing the summary hash of the BPT.  By
// being in the interface, it does eliminate some book keeping where
// a node easily has L and R nodes, but doesn't know if those are Node or
// Value instances.
func (n *BptNode) GetHash() []byte {
	return append([]byte{}, n.Hash[:]...) //   The Hash held by a node is a summary
} //                                           hash of everything above it.

// Marshal
// Serialize the fields of the Node.  Note this doesn't do too much
// towards fitting the node into the actual BPT, but it helps a bit.
//
// See (p *BPT)MarshalByteBlock
func (n *BptNode) Marshal() (data []byte) {
	data = append(data, n.NodeKey[:]...)
	data = append(data, byte(n.Height))
	data = append(data, n.Hash[:]...)
	return data
}

// UnMarshal
// Deserialize the fields of the Node.  See (p *BPT)UnMarshalByteBlock
func (n *BptNode) UnMarshal(data []byte) []byte {
	keySlice, data := data[:32], data[32:]
	copy(n.NodeKey[:], keySlice)
	n.Height, data = int(data[0]), data[1:]
	hashSlice, data := data[:32], data[32:]
	copy(n.Hash[:], hashSlice)
	return data
}
