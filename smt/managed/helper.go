// For key value stores where buckets are not supported, we add a byte to the
// key to represent a bucket. For now, all buckets are hard coded, but we could
// change that in the future.
//
// Buckets are not really enough to index everything we wish to index.  So
// we have labels as well.  Labels are shifted 8 bits left, so they can be
// combined with the buckets to create a unique key.
//
// This allows us to put the raw directory block at DBlockBucket+L_raw, and meta data
// about the directory block at DBlockBucket+MetaLabel
package managed

import (
	"encoding/binary"
)

// This Stateful Merkle Tree implementation handles 256 bit hashes
type Hash [32]byte

// Bytes
// Return a []byte for the Hash
func (h Hash) Bytes() []byte {
	return h[:]
}

// Extract
// Pull out a hash from a byte slice, and return the remaining bytes
func (h *Hash) Extract(data []byte) []byte {
	copy((*h)[:], data[:32])
	return data[32:]
}

// Copy
// Make a copy of a Hash (so the caller cannot modify the original version)
func (h Hash) Copy() Hash {
	return h
}

// Copy
// Make a copy of a Hash (so the caller cannot modify the original version)
func (h Hash) CopyAndPoint() *Hash {
	return &h
}

// Combine
// Hash this hash (the left hash) with the given right hash to produce a new hash
func (h Hash) Combine(hf func(data []byte) Hash, right Hash) Hash {
	return hf(append(h[:], right[:]...)) // Process the left side, i.e. v from this position in c.MD
}

// SliceBytes
// Append a Uvarint length infront of a slice, effectively converting a slice to a counted string
func SliceBytes(slice []byte) []byte {
	var varInt [16]byte                                              // Buffer to hold a Uvarint
	countOfBytes := binary.PutUvarint(varInt[:], uint64(len(slice))) // calculate the Uvarint of the len of the slice
	counted := append(varInt[:countOfBytes], slice...)               // Now put the Uvarint right in front of the slice
	return counted                                                   // Return the resulting counted string
}

// BytesSlice
// Convert a counted byte array (which is a count followed by the byte values) to a slice.  We return what is
// left of the data once the counted byte array is removed
func BytesSlice(data []byte) (slice []byte, data2 []byte) {
	countOfBytes, count := binary.Uvarint(data) // Get the number of bytes in the slice, and count of bytes used for the count
	data = data[count:]
	slice = append(slice, data[:countOfBytes]...)
	data = data[countOfBytes:]
	return slice, data
}
