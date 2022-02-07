package managed

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// BlockIndex
// Holds a mapping of the BlockIndex to the MainIndex and PendingIndex that mark the end of the block
type BlockIndex struct {
	BlockIndex   uint64 // index of the block
	MainIndex    uint64 // index of the last element in the main chain
	PendingIndex uint64 // index of the last element in the Pending chain
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
	data = append(common.Uint64Bytes(b.BlockIndex), common.Uint64Bytes(b.MainIndex)...)
	data = append(data, common.Uint64Bytes(b.PendingIndex)...)
	return data
}

// UnMarshal
// Extract a BlockIndex from a given slice.  Return the remaining slice
func (b *BlockIndex) UnMarshal(data []byte) (newData []byte) {
	b.BlockIndex, data = common.BytesUint64(data)
	b.MainIndex, data = common.BytesUint64(data)
	b.PendingIndex, data = common.BytesUint64(data)
	return data
}

func EndBlock(manager *MerkleManager, cID []byte, blockIndex uint64) error {
	oldCid := manager.key                   // Get the old cid
	defer func() { manager.key = oldCid }() // and restore it when done

	if len(cID) != 32 {
		return fmt.Errorf("chainID is not 32 bytes in length %x", cID)
	}
	pID := cID           // calculate the pending cid
	pID[31] += 1         //
	bIdx := cID          // calculate the block index cid
	bIdx[31] += 2        //
	b := new(BlockIndex) // allocate a block index

	err := manager.SetKey(storage.MakeKey(cID[:]))
	if err != nil {
		return err
	}
	b.MainIndex = uint64(manager.MS.Count)

	err = manager.SetKey(storage.MakeKey(pID[:]))
	if err != nil {
		return err
	}
	b.PendingIndex = uint64(manager.MS.Count)

	err = manager.SetKey(storage.MakeKey(bIdx[:]))
	if err != nil {
		return err
	}
	b.BlockIndex = uint64(manager.MS.Count)

	data := b.Marshal()
	blkIdxHash := manager.MS.HashFunction(data)
	manager.AddHash(blkIdxHash)
	manager.Manager.Put(storage.MakeKey(blkIdxHash), data)

	return nil
}
