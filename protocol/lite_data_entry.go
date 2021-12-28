package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type LiteDataEntry struct {
	//Reserved byte //this was the factom version that never was not zero and is safely assumed as a byte
	ChainId [32]byte
	ExtId   [][]byte
	Data    []byte
}

func ComputeLiteEntryHash(chainId []byte, extIds [][]byte, data []byte) []byte {

	return nil
}

func (e *LiteDataEntry) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer

	empty := [32]byte{}
	if bytes.Equal(e.ChainId[:], empty[:]) {
		return nil, fmt.Errorf("missing ChainID")
	}

	// Header, version byte 0x00
	data := make([]byte, totalSize)
	b.WriteByte(0)
	b.Write(e.ChainId[:])
	b.Write(binary.BigEndian.PutUint16())
	i += copy(data[i:], e.ChainID[:])
	binary.BigEndian.PutUint16(data[i:i+2],
		uint16(totalSize-len(e.Content)-EntryHeaderSize))
	i += 2

	// Payload
	for _, extID := range e.ExtIDs {
		n := len(extID)
		binary.BigEndian.PutUint16(data[i:i+2], uint16(n))
		i += 2
		i += copy(data[i:], extID)
	}
	copy(data[i:], e.Content)

	return data, nil
}
