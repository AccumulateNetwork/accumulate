// Copyright 2022 The Accumulate Authors
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
package managed

import (
	"bytes"
	"crypto/sha256"

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
func (h Hash) Combine(hf HashFunc, right Hash) Hash {
	return hf(append(h.Copy(), right[:]...)) // Process the left side, i.e. v from this position in c.MD
}

func Sha256(b []byte) Hash {
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
