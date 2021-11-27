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
	"bytes"
	"crypto/sha256"

	"github.com/AccumulateNetwork/accumulate/internal/encoding"
)

// This Stateful Merkle Tree implementation handles 256 bit hashes
type Hash []byte

func (h Hash) Bytes32() [32]byte {
	if len(h) != 32 {
		panic("hash is not 32 bytes")
	}
	var g [32]byte
	copy(g[:], h)
	return g
}

func (h Hash) Bytes() []byte { return h }

// Copy
// Make a copy of a Hash (so the caller cannot modify the original version)
func (h Hash) Copy() Hash {
	if h == nil {
		return nil
	}
	g := make(Hash, len(h))
	copy(g, h)
	return g
}

func (h Hash) Equal(g Hash) bool { return bytes.Equal(h, g) }

// Combine
// Hash this hash (the left hash) with the given right hash to produce a new hash
func (h Hash) Combine(hf func(data []byte) Hash, right Hash) Hash {
	return hf(append(h.Copy(), right[:]...)) // Process the left side, i.e. v from this position in c.MD
}

func Sha256(b []byte) Hash {
	h := sha256.Sum256(b)
	return h[:]
}

func (h Hash) BinarySize() int {
	return encoding.BytesBinarySize(h)
}

func (h Hash) MarshalBinary() ([]byte, error) {
	return encoding.BytesMarshalBinary(h), nil
}

func (h *Hash) UnmarhsalBinary(b []byte) error {
	v, err := encoding.BytesUnmarshalBinary(b)
	*h = v
	return err
}

type HashList []Hash

func (h HashList) BinarySize() int {
	s := encoding.UvarintBinarySize(uint64(len(h)))
	for _, h := range h {
		s += h.BinarySize()
	}
	return s
}

func (h HashList) MarshalBinary() ([]byte, error) {
	b := encoding.UvarintMarshalBinary(uint64(len(h)))
	for _, h := range h {
		c, _ := h.MarshalBinary()
		b = append(b, c...)
	}
	return b, nil
}

func (h *HashList) UnmarhsalBinary(b []byte) error {
	l, err := encoding.UvarintUnmarshalBinary(b)
	if err != nil {
		return err
	}
	b = b[encoding.UvarintBinarySize(l):]

	*h = make(HashList, l)
	for i := range *h {
		err = (*h)[i].UnmarhsalBinary(b)
		if err != nil {
			return err
		}
		b = b[(*h)[i].BinarySize():]
	}

	return nil
}
