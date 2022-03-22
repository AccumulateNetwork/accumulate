package protocol

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
)

// ComputeEntryHash
// returns the entry hash given external id's and data associated with an entry
func ComputeEntryHash(data [][]byte) []byte {
	h := make(hash.Hasher, 0, len(data))
	for _, data := range data {
		h.AddBytes(data)
	}
	return h.MerkleHash()
}

const TransactionSizeMax = 10240
const SignatureSizeMax = 1024

func (e *DataEntry) Hash() []byte {
	return ComputeEntryHash(e.Data)
}

//CheckSize is the marshaled size minus the implicit type header,
//returns error if there is too much or no data
func (e *DataEntry) CheckSize() (int, error) {
	b, err := e.MarshalBinary()
	if err != nil {
		return 0, err
	}
	size := len(b)
	if size > TransactionSizeMax {
		return 0, fmt.Errorf("data amount exceeds %v byte entry limit", TransactionSizeMax)
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
