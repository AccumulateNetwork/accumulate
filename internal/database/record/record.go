// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// A Record is a component of a data model.
type Record interface {
	// Resolve resolves the record or a child record.
	Resolve(key Key) (Record, Key, error)
	// IsDirty returns true if the record has been modified.
	IsDirty() bool
	// Commit writes any modifications to the store.
	Commit() error
}

// Value records a value.
type Value[T any] interface {
	Record
	Key(int) any
	Get() (T, error)
	GetAs(any) error
	Put(T) error
}

// List records an unordered list of values as a single record.
type List[T any] interface {
	Value[[]T]
	Add(...T) error
}

// Set records an ordered list of values as a single record.
type Set[T any] interface {
	List[T]
	Remove(T) error
	Index(T) (int, error)
	Find(T) (T, error)
}

// Counted records an insertion-ordered list of values as separate records plus
// a record for the count.
type Counted[T any] interface {
	Record
	Get(int) (T, error)
	Put(T) error
	Count() (int, error)
	GetAll() ([]T, error)
	Last() (int, T, error)
	Overwrite([]T) error
}

// NewValue returns a new value using the given encodable value.
func NewValue[T any](logger log.Logger, store Store, key Key, name string, allowMissing bool, ev encodableValue[T]) Value[T] {
	return newValue(logger, store, key, name, allowMissing, ev)
}

// NewList returns a new list using the given encoder and comparison.
func NewList[T any](logger log.Logger, store Store, key Key, namefmt string, encoder encodableValue[T]) List[T] {
	return newList(logger, store, key, namefmt, encoder)
}

// NewSet returns a new set using the given encoder and comparison.
func NewSet[T any](logger log.Logger, store Store, key Key, namefmt string, encoder encodableValue[T], cmp func(u, v T) int) Set[T] {
	return newSet(logger, store, key, namefmt, encoder, cmp)
}

// NewCounted returns a new counted using the given encodable value type.
func NewCounted[T any](logger log.Logger, store Store, key Key, namefmt string, new func() encodableValue[T]) Counted[T] {
	return newCounted(logger, store, key, namefmt, new)
}

// A Key is the key for a record.
type Key []interface{}

// Append creates a child key of this key.
func (k Key) Append(v ...interface{}) Key {
	l := make(Key, len(k)+len(v))
	n := copy(l, k)
	copy(l[n:], v)
	return l
}

// Hash converts the record key to a storage key.
func (k Key) Hash() storage.Key {
	return storage.MakeKey(k...)
}

// String returns a human-readable string for the key.
func (k Key) String() string {
	s := make([]string, len(k))
	for i, v := range k {
		switch v := v.(type) {
		case []byte:
			s[i] = hex.EncodeToString(v)
		case [32]byte:
			s[i] = hex.EncodeToString(v[:])
		default:
			s[i] = fmt.Sprint(v)
		}
	}
	return strings.Join(s, ".")
}

// MarshalJSON is implemented so keys are formatted nicely by zerolog.
func (k Key) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// A ValueReader holds a readable value.
type ValueReader interface {
	// GetValue returns the value.
	GetValue() (value encoding.BinaryValue, version int, err error)
}

// A ValueWriter holds a writable value.
type ValueWriter interface {
	// LoadValue stores the value of the reader into the receiver.
	LoadValue(value ValueReader, put bool) error
	// LoadBytes unmarshals a value from bytes into the receiver.
	LoadBytes(data []byte) error
}

// A Store loads and stores values.
type Store interface {
	// GetValue loads the value from the underlying store and writes it. Byte
	// stores call LoadBytes(data) and value stores call LoadValue(v, false).
	GetValue(key Key, value ValueWriter) error
	// PutValue gets the value from the reader and stores it. A byte store
	// marshals the value and stores the bytes. A value store finds the
	// appropriate value and calls LoadValue(v, true).
	PutValue(key Key, value ValueReader) error
}
