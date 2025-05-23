// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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
package database

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
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

type HashFunc func([]byte) Hash

// Combine
// Hash this hash (the left hash) with the given right hash to produce a new hash
func (h Hash) Combine(right Hash) Hash {
	var b [64]byte
	copy(b[:], h)
	copy(b[32:], right)
	return doSha(b[:]) // Process the left side, i.e. v from this position in c.MD
}

func doSha(b []byte) Hash {
	h := sha256.Sum256(b)
	return h[:]
}

func (h Hash) BinarySize() int {
	return len(encoding.MarshalBytes(h))
}

func (h Hash) MarshalBinary() ([]byte, error) {
	return encoding.MarshalBytes(h), nil
}

func (h *Hash) UnmarhsalBinary(b []byte) error {
	v, err := encoding.UnmarshalBytes(b)
	*h = v
	return err
}

type SparseHashList [][]byte

func (l SparseHashList) Copy() SparseHashList {
	m := make(SparseHashList, len(l))
	for i, v := range l {
		m[i] = Hash(v).Copy()
	}
	return m
}

func (l SparseHashList) BinarySize(height int64) int {
	var n int
	for i, h := range l {
		// If the bit is not set, skip that entry
		if height&(1<<i) == 0 {
			continue
		}

		n += len(encoding.MarshalBytes(h))
	}
	return n
}

func (l SparseHashList) MarshalBinary(height int64) ([]byte, error) {
	var data []byte

	// For each bit in height
	for i := 0; height > 0; i++ {
		if i >= len(l) {
			return nil, fmt.Errorf("missing hash at [%d]", i)
		}

		// If the bit is set, record the hash, otherwise ignore it (it is nil)
		if height&1 > 0 {
			data = append(data, encoding.MarshalBytes(l[i])...)
		}

		// Shift height so we can check the next bit
		height = height >> 1
	}

	return data, nil
}

func (l *SparseHashList) UnmarshalBinary(height int64, data []byte) error {
	// Count the number of bits required to store the height
	n := bits.Len64(uint64(height))

	// Clear the list and ensure it has sufficient capacity
	*l = append((*l)[:0], make(SparseHashList, n)...)

	for i := range *l {
		// If the bit is not set, skip that entry
		if height&(1<<i) == 0 {
			continue
		}

		// If the bit is set, then extract the next hash
		var err error
		(*l)[i], err = encoding.UnmarshalBytes(data)
		if err != nil {
			return err
		}

		// Advance data by the hash size
		data = data[len(encoding.MarshalBytes((*l)[i])):]
	}

	return nil
}

type HashList [][]byte

func (l HashList) BinarySize() int {
	s := len(encoding.MarshalUint(uint64(len(l))))
	for _, h := range l {
		s += Hash(h).BinarySize()
	}
	return s
}

func (l HashList) MarshalBinary() ([]byte, error) {
	b := encoding.MarshalUint(uint64(len(l)))
	for _, h := range l {
		c, _ := Hash(h).MarshalBinary()
		b = append(b, c...)
	}
	return b, nil
}

func (l *HashList) UnmarhsalBinary(b []byte) error {
	n, err := encoding.UnmarshalUint(b)
	if err != nil {
		return err
	}
	b = b[len(encoding.MarshalUint(n)):]

	*l = make(HashList, n)
	for i := range *l {
		err = (*Hash)(&(*l)[i]).UnmarhsalBinary(b)
		if err != nil {
			return err
		}
		b = b[(*Hash)(&(*l)[i]).BinarySize():]
	}

	return nil
}
