package protocol

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"math"
)

func NewFactomDataEntry() *FactomDataEntry {
	return new(FactomDataEntry)
}

//ComputeFactomEntryHashForAccount will compute the entry hash from an entry.
//If accountId is nil, then entry will be used to construct an account id,
//and it assumes the entry will be the first entry in the chain
func ComputeFactomEntryHashForAccount(accountId []byte, entry [][]byte) ([]byte, error) {
	lde := FactomDataEntry{}
	if entry == nil {
		return nil, fmt.Errorf("cannot compute lite entry hash, missing entry")
	}
	if len(entry) > 0 {
		lde.Data = entry[0]
		lde.ExtIds = entry[1:]
	}

	if accountId == nil && entry != nil {
		//if we don't know the chain id, we compute one off of the entry
		copy(lde.AccountId[:], ComputeLiteDataAccountId(lde.Wrap()))
	} else {
		copy(lde.AccountId[:], accountId)
	}
	return lde.Hash(), nil
}

// ComputeFactomEntryHash returns the Entry hash of data for a Factom data entry
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry-hash
func ComputeFactomEntryHash(data []byte) []byte {
	sum := sha512.Sum512(data)
	saltedSum := make([]byte, len(sum)+len(data))
	i := copy(saltedSum, sum[:])
	copy(saltedSum[i:], data)
	h := sha256.Sum256(saltedSum)
	return h[:]
}

func (e *FactomDataEntry) Hash() []byte {
	d, err := e.MarshalBinary()
	if err != nil {
		// TransactionPayload.MarshalBinary should never return an error, but
		// better a panic then a silently ignored error.
		panic(err)
	}
	return ComputeFactomEntryHash(d)
}

func (e *FactomDataEntryWrapper) GetData() [][]byte {
	return append([][]byte{e.Data}, e.ExtIds...)
}

// MarshalBinary marshal the FactomDataEntry in accordance to
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e *FactomDataEntry) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer

	d := [32]byte{}
	if e.AccountId == ([32]byte{}) {
		return nil, fmt.Errorf("missing ChainID")
	}

	// Header, version byte 0x00
	b.WriteByte(0)
	b.Write(e.AccountId[:])

	// Payload
	var ex bytes.Buffer

	//ExtId's are data entries 1..N if applicable
	for _, data := range e.ExtIds {
		n := len(data)
		binary.BigEndian.PutUint16(d[:], uint16(n))
		ex.Write(d[:2])
		ex.Write(data)
	}
	binary.BigEndian.PutUint16(d[:], uint16(ex.Len()))
	b.Write(d[:2])
	b.Write(ex.Bytes())

	b.Write(e.Data)

	return b.Bytes(), nil
}

// LiteEntryHeaderSize is the exact length of an Entry header.
const LiteEntryHeaderSize = 1 + // version
	32 + // chain id
	2 // total len

// // LiteEntryMaxTotalSize is the maximum encoded length of an Entry.
// const LiteEntryMaxTotalSize = TransactionSizeMax + LiteEntryHeaderSize

// UnmarshalBinary unmarshal the FactomDataEntry in accordance to
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e *FactomDataEntry) UnmarshalBinary(data []byte) error {
	if len(data) < LiteEntryHeaderSize || len(data) > math.MaxInt32 {
		return fmt.Errorf("malformed entry header")
	}

	// Ignore version
	data = data[1:]

	// Get account ID
	copy(e.AccountId[:], data[:32])
	data = data[32:]

	// Get total ExtIDs size
	totalExtIdSize := binary.BigEndian.Uint16(data[:2])
	data = data[2:]

	if int(totalExtIdSize) > len(data) || totalExtIdSize == 1 {
		return fmt.Errorf("malformed entry payload")
	}

	// Reset fields if present
	e.Data = nil
	e.ExtIds = nil

	// Get entry data
	e.Data = data[totalExtIdSize:]
	data = data[:totalExtIdSize]

	for len(data) > 0 {
		// Get ExtID size
		if len(data) < 2 {
			return fmt.Errorf("malformed extId")
		}
		extIdSize := binary.BigEndian.Uint16(data[:2])
		data = data[2:]
		if int(extIdSize) > len(data) {
			return fmt.Errorf("malformed extId")
		}

		// Get ExtID data
		e.ExtIds = append(e.ExtIds, data[:extIdSize])
		data = data[extIdSize:]
	}
	return nil
}

func (e *FactomDataEntry) IsValid() error  { return nil }
func (e *FactomDataEntry) Wrap() DataEntry { return &FactomDataEntryWrapper{FactomDataEntry: *e} }
