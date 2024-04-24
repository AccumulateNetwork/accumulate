package blockchainDB

import (
	"encoding/binary"
	"fmt"
)

type DBBKey struct {
	Offset uint64
	Length uint64
}

// Bytes
// Writes out the address with the offset and length of the DBBKey
func (d *DBBKey) Bytes(address [32]byte) []byte {
	var b [48]byte
	copy(b[:], address[:])
	binary.BigEndian.PutUint64(b[32:], d.Offset)
	binary.BigEndian.PutUint64(b[40:], d.Length)
	return b[:]
}

// GetDBBKey
// Converts a 48 byte slice into an Address and a DBBKey
func GetDBBKey(data []byte) (address [32]byte, dBBKey *DBBKey, err error) {
	dBBKey = new(DBBKey)
	address, err = dBBKey.Unmarshal(data)
	return address, dBBKey, err
}

// Unmarshal
// Returns the address and the DBBKey from a slice of bytes
func (d *DBBKey) Unmarshal(data []byte) (address [32]byte, err error) {
	if len(data) < 48 {
		return address, fmt.Errorf("data source is short %d", len(data))
	}
	copy(address[:], data[:32])
	d.Offset = binary.BigEndian.Uint64(data[32:])
	d.Length = binary.BigEndian.Uint64(data[40:])
	return address, nil
}
