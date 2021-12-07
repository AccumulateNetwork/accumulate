package protocol

import (
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/smt/managed"
)

//func (e *DataEntry) Equal(rhs DataEntry) bool {
//	return true
//}

type DataAccountStateCache struct {
	DataAccount

	//this is part of the data cache and is not persisted with the state.
	entryHash []byte
	data      []byte
}

func NewDataAccountStateCache(da *DataAccount, entryHash []byte, data []byte) *DataAccountStateCache {
	cachedState := new(DataAccountStateCache)
	cachedState.DataAccount = *da
	cachedState.entryHash = entryHash
	cachedState.data = data
	return cachedState
}

func (d *DataAccountStateCache) GetData() []byte {
	return d.data
}

func (d *DataAccountStateCache) GetEntryHash() []byte {
	return d.entryHash
}

// ComputeEntryHash
// returns the entry hash given external id's and data associated with an entry
func ComputeEntryHash(data [][]byte) []byte {
	smt := managed.MerkleState{}
	//add the external id's to the merkle tree
	for i := range data {
		h := sha256.Sum256(data[i])
		smt.AddToMerkleTree(h[:])
	}
	//return the entry hash
	return smt.GetMDRoot().Bytes()
}

const WriteDataMax = 10240

func (e *DataEntry) Hash() []byte {
	return ComputeEntryHash(append(e.ExtIds, e.Data))
}

//CheckSize is the marshaled size minus the implicit type header,
//returns error if there is too much or no data
func (e *DataEntry) CheckSize() (int, error) {
	size := e.BinarySize()
	if size > WriteDataMax {
		return 0, fmt.Errorf("data amount exceeds %v byte entry limit", WriteDataMax)
	}
	if size <= 0 {
		return 0, fmt.Errorf("no data provided for WriteData")
	}
	return size, nil
}

//Cost will return the number of credits to be used for the data write
func (e *DataEntry) Cost() (int, error) {
	size, err := e.CheckSize()
	if err != nil {
		return 0, err
	}
	return FeeWriteData.AsInt() * (size/256 + 1), nil
}
