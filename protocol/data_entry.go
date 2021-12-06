package protocol

import (
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/encoding"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/types"
)

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

func (w *WriteData) Hash() []byte {
	return ComputeEntryHash(append(w.ExtIds, w.Data))
}

//CheckSize is the marshaled size minus the implicit type header,
//returns error if there is too much or no data
func (w *WriteData) CheckSize() (int, error) {
	size := w.BinarySize() - len(encoding.UvarintMarshalBinary(types.TxTypeWriteData.ID()))
	if size > WriteDataMax {
		return 0, fmt.Errorf("data amount exceeds %v byte entry limit", WriteDataMax)
	}
	if size <= 0 {
		return 0, fmt.Errorf("no data provided for WriteData")
	}
	return size, nil
}

//Cost will return the number of credits to be used for the data write
func (w *WriteData) Cost() (int, error) {
	size, err := w.CheckSize()
	if err != nil {
		return 0, err
	}
	return FeeWriteData.AsInt() * (size/256 + 1), nil
}
