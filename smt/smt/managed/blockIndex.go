package managed

import "github.com/AccumulateNetwork/SMT/smt/storage"

// BlockIndex
// Holds a mapping of the BlockIndex to the MainIndex and PendingIndex that mark the end of the block
type BlockIndex struct {
	BlockIndex   int64 // index of the block
	MainIndex    int64 // index of the last element in the main chain
	PendingIndex int64 // index of the last element in the Pending chain
}

// Marshal
// serialize a BlockIndex into a slice of data
func (b *BlockIndex) Marshal() (data []byte) {
	data = append(storage.Int64Bytes(b.BlockIndex), storage.Int64Bytes(b.MainIndex)...)
	data = append(data, storage.Int64Bytes(b.PendingIndex)...)
	return data
}

// UnMarshal
// Extract a BlockIndex from a given slice.  Return the remaining slice
func (b *BlockIndex) UnMarshal(data []byte) (newData []byte) {
	b.BlockIndex, data = storage.BytesInt64(data)
	b.MainIndex, data = storage.BytesInt64(data)
	b.PendingIndex, data = storage.BytesInt64(data)
	return data
}
