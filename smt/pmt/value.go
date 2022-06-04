package pmt

import "bytes"

// Value
// Every Account has a BPT entry with the hash of the state of the account.
// The BPTKey is constructed from:
//     the hash of the ADI URL (ADI)
//     the hash of the account URL  (Account)
//     a BPT key built from the ADI and Account (BPTKey)
//     the hash of the account state (Hash)
type Value struct {
	ADI     [32]byte // ADI of the account
	Account [32]byte // hash of the account URL
	Key     [32]byte // Key used to put the current state of the account in the BPT
	Hash    [32]byte // The current value for the key
}

const ValueLen = 32*4

// Copy
// Copy the contents of the given Value into this instance of the value
func (v *Value) Copy(value *Value) {
	copy(v.ADI[:] ,value.ADI[:])
	copy(v.Account[:] ,value.Account[:])
	copy(v.Key[:] ,value.Key[:])
	copy(v.Hash[:] ,value.Hash[:])
}

// Node
// Returns true if this is a Node, otherwise it is a value
func (v *Value) T() int {
	return TValue
}

// GetHash
// Returns the hash as a slice
func (v *Value) GetHash() []byte {
	return append([]byte{}, v.Hash[:]...)
}

// Marshal
// Return the concatenation of the Key and Hash of the value
func (v *Value) Marshal() (data []byte) {
	data = append(v.ADI[:], v.Account[:]...)
	data = append(data, v.Key[:]...)
	data = append(data, v.Hash[:]...)
	return data
}

// UnMarshal
// Load the Value with the state marshalled to the given data slice
func (v *Value) UnMarshal(data []byte) []byte {
	copy(v.ADI[:], data[:32])
	data = data[32:]
	copy(v.Account[:], data[:32])
	data = data[32:]
	copy(v.Key[:], data[:32])
	data = data[32:]
	copy(v.Hash[:], data[:32])
	data = data[32:]
	return data
}

// Equal
// Return true if a given Entry is a Value instance, and has the same
// Key and Hash as this Value
func (v *Value) Equal(entry Entry) (equal bool) {

	value, ok := entry.(*Value) //                           The entry we are considering must be a node
	switch {
	case !ok:
		return false
	case !bytes.Equal(v.ADI[:], value.ADI[:]):
		return false
	case !bytes.Equal(v.Account[:], value.Account[:]):
		return false
	case !bytes.Equal(v.Key[:], value.Key[:]):
		return false
	case !bytes.Equal(v.Hash[:], value.Hash[:]):
		return false
	}
	return true
}
