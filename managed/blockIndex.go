package managed

// BlockIndex
// Holds a mapping of the BlockIndex to the Element that marks the end of the block
type BlockIndex struct {
	BlockIndex   int64 // index of the block
	ElementIndex int64 // index of the last element in the block
}

// Marshal
// serialize a BlockIndex into a slice of data
func (b *BlockIndex) Marshal() []byte {
	return append(Int64Bytes(b.BlockIndex), Int64Bytes(b.ElementIndex)...)
}

// UnMarshal
// Extract a blockindex from a given slice.  Return the remaining slice
func (b *BlockIndex) UnMarshal(data []byte) (newData []byte) {
	b.BlockIndex, data = BytesInt64(data)
	b.ElementIndex, data = BytesInt64(data)
	return data
}
