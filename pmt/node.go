package pmt

// Note that the tree considered here grows up by convention here, where
// parent nodes are at the bottom, and leaves are at the top. Obviously
// mapping up to down and down to up is valid if a need to have the tree
// grow down is viewed as important.

// Node
// A node in our binary patricia/merkle tree
type Node struct {
	ID     int      // Node Count
	Height int      // Root is 0. Under root is 1. Under under root is 2, etc.
	Hash   [32]byte // This is the summary hash for the tree
	left   Entry    // The hash of the child left and up the tree, bit is zero
	right  Entry    // the hash to the child right and up the tree, bit is one
	parent *Node    // the parent node "below" this node
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
func (n *Node) GetID() int {
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
