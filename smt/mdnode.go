package smt

type MDNode struct {
	Type           uint8  // Type of data in this chain, drives validation
	SequenceNumber uint32 // sequence number of MDNodes in this chain
	Previous       Hash   // Hash of the previous MDNode in this chain; zeros if first block
	TotalEntries   uint64 // Total number of entries at the start of this block in this chain
	MDSTate        []Hash // Hashes in our intermediate state at the start of this block
	MDRoot         Hash   // MDRoot of all the data in this chain up to and not including this MDNode
	Hashes         []Hash // Hashes in this block
}

// SameAs
// Compare all the data in two nodes to determine that they are the same.  Any difference, and they are false
func (n *MDNode) SameAs(n2 *MDNode) bool {
	if n.Type != n2.Type { // Check the type
		return false
	}
	if n.SequenceNumber != n2.SequenceNumber { // Check the sequence number
		return false
	}
	if n.Previous != n2.Previous {
		return false
	}
	if n.TotalEntries != n2.TotalEntries {
		return false
	}
	if n.MDRoot != n2.MDRoot { // Check the MDRoot
		return false
	}
	if len(n.Hashes) != len(n2.Hashes) {
		return false
	} // Check the length of the hashes
	for i, n := range n.Hashes { // Check each of the hashes
		if n != n2.Hashes[i] {
			return false
		}
	}
	return true
}

func (n *MDNode) Bytes() (data []byte) {
	data = append(data, byte(n.Type))
	data = append(data, Uint32Bytes(n.SequenceNumber)...)
	data = append(data, n.MDRoot.Bytes()...)
	data = append(data, Uint32Bytes(uint32(len(n.Hashes)))...)
	for _, h := range n.Hashes {
		data = append(data, h.Bytes()...)
	}
	return data
}

func (n *MDNode) Extract(data []byte) {
	n.Hashes = n.Hashes[:0] // Clear any old hashes
	n.Type, data = data[0], data[1:]
	n.SequenceNumber, data = BytesUint32(data)
	data = n.MDRoot.Extract(data)
	var numHashes, i uint32
	numHashes, data = BytesUint32(data)
	for i = 0; i < numHashes; i++ {
		var h Hash
		data = h.Extract(data)
		n.Hashes = append(n.Hashes, h)
	}
}

// CompressState
// First check to make sure the current state matches the total number of entries at the start of this
// block.
func (n *MDNode) CompressState() (state []Hash) {
	return
}
