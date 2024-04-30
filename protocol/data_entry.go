// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

const TransactionSizeMax = 20480 // Must be over 10k to accommodate Factom entries
const SignatureSizeMax = 1024

type DataEntryType uint64

type DataEntry interface {
	encoding.UnionValue
	Type() DataEntryType
	Hash() []byte
	GetData() [][]byte
}

type WithDataEntry interface {
	GetDataEntry() DataEntry
}

func (w *WriteData) GetDataEntry() DataEntry          { return w.Entry }
func (w *WriteDataTo) GetDataEntry() DataEntry        { return w.Entry }
func (w *SyntheticWriteData) GetDataEntry() DataEntry { return w.Entry }
func (w *SystemWriteData) GetDataEntry() DataEntry    { return w.Entry }

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

func (e *DoubleHashDataEntry) Hash() []byte {
	h := make(hash.Hasher, 0, len(e.Data))
	for _, data := range e.Data {
		h.AddBytes(data)
	}

	// Double hash the Merkle root
	hh := sha256.Sum256(h.MerkleHash())
	return hh[:]
}

func (e *DoubleHashDataEntry) GetData() [][]byte {
	return e.Data
}

func (e *ProxyDataEntry) Hash() []byte {
	e.EnsureHashes()
	h := make(hash.Hasher, 0, len(e.Hashes))
	for _, hash := range e.Hashes {
		h.AddHash2(hash)
	}

	// Double hash the Merkle root
	hh := sha256.Sum256(h.MerkleHash())
	return hh[:]
}

func (e *ProxyDataEntry) GetData() [][]byte {
	// Data may be unavailable but hashes will always be present or calculable,
	// so we'll return those
	e.EnsureHashes()
	r := make([][]byte, len(e.Hashes))
	for i, hash := range e.Hashes {
		r[i] = hash[:]
	}
	return r
}

func (e *ProxyDataEntry) EnsureHashes() {
	// This will race if called from multiple routines
	if len(e.Hashes) != 0 || len(e.Data) == 0 {
		return
	}

	e.Hashes = make([][32]byte, len(e.Data))
	for i, data := range e.Data {
		e.Hashes[i] = sha256.Sum256(data)
	}
}

func (e *ProxyDataEntry) Verify() error {
	if len(e.Hashes) == 0 || len(e.Data) == 0 {
		return nil
	}

	if len(e.Hashes) != len(e.Data) {
		return errors.BadRequest.With("len(hashes) != len(data)")
	}

	for i, h := range e.Hashes {
		if h != sha256.Sum256(e.Data[i]) {
			return errors.BadRequest.WithFormat("hash[%d] = H(data[%[1]d])", i)
		}
	}
	return nil
}

// CheckDataEntrySize is the marshaled size minus the implicit type header,
// returns error if there is too much or no data
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

// Cost will return the number of credits to be used for the data write
func DataEntryCost(e DataEntry) (uint64, error) {
	size, err := CheckDataEntrySize(e)
	if err != nil {
		return 0, err
	}
	return FeeData.AsUInt64() * uint64(size/256+1), nil
}
