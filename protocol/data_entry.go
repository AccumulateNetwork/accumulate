// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
)

type DataEntryType uint64

type DataEntry interface {
	encoding.BinaryValue
	Type() DataEntryType
	Hash() []byte
	GetData() [][]byte
}

const TransactionSizeMax = 20480 // Must be over 10k to accommodate Factom entries
const SignatureSizeMax = 1024

// Hash
// returns the entry hash given external id's and data associated with an entry
func (e *AccumulateDataEntry) Hash() []byte {
	h := make(hash.Hasher, 0, len(e.Data))
	for _, data := range e.Data {
		h.AddBytes(data)
	}
	return h.MerkleHash()
}

func (e *AccumulateDataEntry) GetData() [][]byte {
	return e.Data
}

//CheckDataEntrySize is the marshaled size minus the implicit type header,
//returns error if there is too much or no data
func CheckDataEntrySize(e DataEntry) (int, error) {
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
func DataEntryCost(e DataEntry) (uint64, error) {
	size, err := CheckDataEntrySize(e)
	if err != nil {
		return 0, err
	}
	return FeeData.AsUInt64() * uint64(size/256+1), nil
}
