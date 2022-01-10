package protocol

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
)

type LiteDataEntry struct {
	ChainId [32]byte
	DataEntry
}

// ComputeLiteEntryHash returns the Entry hash of data for a lite chain
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry-hash
func ComputeLiteEntryHash(data []byte) []byte {
	sum := sha512.Sum512(data)
	saltedSum := make([]byte, len(sum)+len(data))
	i := copy(saltedSum, sum[:])
	copy(saltedSum[i:], data)
	h := sha256.Sum256(saltedSum)
	return h[:]
}

func (e *LiteDataEntry) Hash() ([]byte, error) {
	d, err := e.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return ComputeLiteEntryHash(d), nil
}

// MarshalBinary marshal the LiteDataEntry in accordance to
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e *LiteDataEntry) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer

	d := [32]byte{}
	if bytes.Equal(e.ChainId[:], d[:]) {
		return nil, fmt.Errorf("missing ChainID")
	}

	// Header, version byte 0x00
	b.WriteByte(0)
	b.Write(e.ChainId[:])

	// Payload
	var ex bytes.Buffer
	for _, extID := range e.ExtIds {
		n := len(extID)
		binary.BigEndian.PutUint16(d[:], uint16(n))
		ex.Write(d[:2])
		ex.Write(extID)
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

// LiteEntryMaxTotalSize is the maximum encoded length of an Entry.
const LiteEntryMaxTotalSize = WriteDataMax + LiteEntryHeaderSize

// UnmarshalBinary unmarshal the LiteDataEntry in accordance to
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#entry
func (e *LiteDataEntry) UnmarshalBinary(data []byte) error {

	if len(data) < LiteEntryHeaderSize || len(data) > LiteEntryMaxTotalSize {
		return fmt.Errorf("malformed entry header")
	}

	copy(e.ChainId[:], data[1:33])

	totalExtIdSize := binary.BigEndian.Uint16(data[33:35])

	if int(totalExtIdSize) > len(data)-LiteEntryHeaderSize || totalExtIdSize == 1 {
		return fmt.Errorf("malformed entry payload")
	}

	j := LiteEntryHeaderSize

	//reset the extId's if present
	e.ExtIds = e.ExtIds[0:0]
	for n := 0; n < int(totalExtIdSize); {
		extIdSize := binary.BigEndian.Uint16(data[j : j+2])
		if extIdSize > totalExtIdSize {
			return fmt.Errorf("malformed extId")
		}
		j += 2
		e.ExtIds = append(e.ExtIds, data[j:j+int(extIdSize)])
		j += n
	}

	e.Data = data[j:]
	return nil
}
