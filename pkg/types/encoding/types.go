// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"fmt"
	"hash"
	"io"
	"sync"
)

type Error struct {
	E error
}

func (e Error) Error() string { return e.E.Error() }
func (e Error) Unwrap() error { return e.E }

type EnumValueGetter interface {
	GetEnumValue() uint64
}

type EnumValueSetter interface {
	SetEnumValue(uint64) bool
}

type BinaryValue interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	CopyAsInterface() interface{}
	UnmarshalBinaryFrom(io.Reader) error
}

// WritableBinaryValue extends BinaryValue with efficient direct writing.
// Types implementing this interface can write directly to an io.Writer
// without intermediate allocation.
type WritableBinaryValue interface {
	BinaryValue
	WriteTo(w io.Writer) (int64, error)
}

type UnionValue interface {
	BinaryValue
	UnmarshalFieldsFrom(reader *Reader) error
}

// bufferPool is a pool of bytes.Buffer for reducing allocations in MarshalBinary.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// GetBuffer returns a buffer from the pool.
func GetBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(buf *bytes.Buffer) {
	// Don't pool huge buffers to avoid memory bloat
	if buf.Cap() > 1<<20 { // 1MB
		return
	}
	bufferPool.Put(buf)
}

// hashPool is a pool of sha256 hashers for reducing allocations in Hash.
var hashPool = sync.Pool{
	New: func() interface{} {
		return sha256.New()
	},
}

// Hash computes the SHA256 hash of a BinaryValue.
// If the value implements WritableBinaryValue, it writes directly to the hasher
// without intermediate allocation. Otherwise, it falls back to MarshalBinary.
func Hash(m BinaryValue) [32]byte {
	// Fast path: use WriteTo if available
	if w, ok := m.(WritableBinaryValue); ok {
		h := hashPool.Get().(hash.Hash)
		h.Reset()
		defer hashPool.Put(h)

		_, err := w.WriteTo(h)
		if err != nil {
			panic(fmt.Errorf("writing message to hash: %w", err))
		}

		var result [32]byte
		h.Sum(result[:0])
		return result
	}

	// Slow path: allocate via MarshalBinary
	b, err := m.MarshalBinary()
	if err != nil {
		panic(fmt.Errorf("marshaling message: %w", err))
	}
	return sha256.Sum256(b)
}
