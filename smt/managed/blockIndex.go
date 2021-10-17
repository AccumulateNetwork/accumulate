package managed

import (
	"github.com/AccumulateNetwork/accumulated/smt/common"
)

// BlockIndex
// Holds a mapping of the BlockIndex to the MainIndex and PendingIndex that mark the end of the block
type BlockIndex struct {
	BlockIndex   int64 // index of the block
	MainIndex    int64 // index of the last element in the main chain
	PendingIndex int64 // index of the last element in the Pending chain
}

// Equal
// Compares two BlockIndex instances
func (b *BlockIndex) Equal(b2 *BlockIndex) bool {
	if b.BlockIndex != b2.BlockIndex {
		return false
	}
	if b.MainIndex != b2.MainIndex {
		return false
	}
	if b.PendingIndex != b2.PendingIndex {
		return false
	}
	return true
}

// Marshal
// serialize a BlockIndex into a slice of data
func (b *BlockIndex) Marshal() (data []byte) {
	data = append(common.Int64Bytes(b.BlockIndex), common.Int64Bytes(b.MainIndex)...)
	data = append(data, common.Int64Bytes(b.PendingIndex)...)
	return data
}

// UnMarshal
// Extract a BlockIndex from a given slice.  Return the remaining slice
func (b *BlockIndex) UnMarshal(data []byte) (newData []byte) {
	b.BlockIndex, data = common.BytesInt64(data)
	b.MainIndex, data = common.BytesInt64(data)
	b.PendingIndex, data = common.BytesInt64(data)
	return data
}
