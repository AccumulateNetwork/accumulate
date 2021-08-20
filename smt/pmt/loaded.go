package pmt

// Note that the tree considered here grows up by convention here, where
// Parent nodes are at the bottom, and leaves are at the top. Obviously
// mapping up to down and down to up is valid if a need to have the tree
// grow down is viewed as important.

// Node
// A node in our binary patricia/merkle tree
type NotLoaded struct {
}

var _ Entry = new(NotLoaded)

func (n *NotLoaded) T() int {
	return TNotLoaded
}

func (n *NotLoaded) GetHash() []byte {
	return nil
}

func (n *NotLoaded) Marshal() []byte {
	return nil
}
func (n *NotLoaded) UnMarshal(data []byte) []byte {
	return data
}
func (n *NotLoaded) Equal(entry Entry) bool {
	return entry.T() == TNotLoaded
}
