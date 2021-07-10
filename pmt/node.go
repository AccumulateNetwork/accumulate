package pmt

import (
	"bytes"

	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/storage"
)

// Note that the tree considered here grows up by convention here, where
// parent nodes are at the bottom, and leaves are at the top. Obviously
// mapping up to down and down to up is valid if a need to have the tree
// grow down is viewed as important.

// Node
// A node in our binary patricia/merkle tree
type Node struct {
	ID       int64    // Node Count
	PreBytes []byte   // Bytes preceding the block that contains this node
	Height   uint8    // Root is 0. above root is 1. Above above root is 2, etc.
	Hash     [32]byte // This is the summary hash for the tree
	left     Entry    // The hash of the child left and up the tree, bit is zero
	right    Entry    // the hash to the child right and up the tree, bit is one
	parent   *Node    // the parent node "below" this node
}

func (n *Node) Equal(entry Entry) (equal bool) {

	defer func() { //                          If we access a nil, it is because something is missing
		if err := recover(); err != nil { //
			equal = false
		}
	}()

	// We compare only down the BPT.  If we compare both up and down the tree,
	// then the code would loop infinitely.  Certainly we could avoid retracing
	// paths, but if we wish to compare entire BPT trees, we can compare their
	// roots.
	node := entry.(*Node) //                           The entry we are considering must be a node
	switch {
	case !bytes.Equal(n.PreBytes, node.PreBytes):
		return false
	case n.Height != node.Height:
		return false
	case !bytes.Equal(n.Hash[:], node.Hash[:]):
		return false
	case n.left == nil && node.left != nil:
		return false
	case n.right == nil && node.right != nil:
		return false
	case n.left != nil && !n.left.Equal(node.left):
		return false
	case n.right != nil && !n.right.Equal(node.right):
		return false
	}
	return true
}

// T
// Returns true if this node is really a Node, and false if the node
// is really a Value.  We possibly want a way for Node to compress the
// distance to child nodes that have a long single path to leaves just
// because a key matches a number of bits before differentiating.
func (n *Node) T() bool {
	return true
}

// GetID
// Returns the ID for this node.  This is used to compare nodes and serve
// as a key in the BPT.DirtyMap.
func (n *Node) GetID() int64 {
	return n.ID
}

// GetHash
// Returns the Hash value for computing the summary hash of the BPT.  By
// being in the interface, it does eliminate some book keeping where
// a node easily has L and R nodes, but doesn't know if those are Node or
// Value instances.
func (n *Node) GetHash() []byte {
	return n.Hash[:] //             The Hash held by a node is a summary
} //                                hash of everything above it.

// Marshal
// Serialize the fields of the Node.  Note this doesn't do too much
// towards fitting the node into the actual BPT, but it helps a bit.
//
// See (p *BPT)MarshalByteBlock
func (n *Node) Marshal() (data []byte) {
	data = append(data, storage.Int64Bytes(n.ID)...)
	data = append(data, managed.SliceBytes(n.PreBytes)...)
	data = append(data, n.Height)
	data = append(data, n.Hash[:]...)
	return data
}

// UnMarshal
// Deserialize the fields of the Node.  See (p *BPT)UnMarshalByteBlock
func (n *Node) UnMarshal(data []byte) []byte {
	n.ID, data = storage.BytesInt64(data)
	n.PreBytes, data = managed.BytesSlice(data)
	n.Height, data = data[0], data[1:]
	hashSlice, data := managed.BytesSlice(data)
	copy(n.Hash[:], hashSlice)
	return data
}
