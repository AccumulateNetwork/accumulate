// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package database defines the interfaces used for data storage.
package database

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A Key is the key for a record.
type Key = record.Key

func NewKey(v ...any) *Key { return record.NewKey(v...) }

// A Record is a component of a data model.
type Record interface {
	// Key returns the record's key.
	Key() *Key

	// Resolve resolves the record or a child record.
	Resolve(key *Key) (Record, *Key, error)

	// IsDirty returns true if the record has been modified.
	IsDirty() bool

	// Commit writes any modifications to the store.
	Commit() error

	// Walk walks the record.
	//
	// If the record is clean and opts.Changes is set, Walk returns immediately,
	// without calling the callback or recursing. Otherwise if the record is
	// terminal (not composite), Walk calls the callback on the record. The
	// behavior of Walk for a composite record depends on opts.Values, though
	// the first condition (return immediately if clean and opts.Changes is set)
	// still holds. If opts.Values is set, Walk calls the callback for every
	// component of the record but not for the composite record itself. If
	// opts.Values is not set, Walk calls the callback on the composite record.
	// If the callback returns skip = true, Walk returns immediately. Otherwise,
	// Walk calls the callback for every component of the record.
	Walk(opts WalkOptions, fn WalkFunc) error
}

type WalkFunc = func(Record) (skip bool, err error)

type WalkOptions struct {
	// Values indicates the callback should only be called for values, not
	// composite records.
	Values bool

	// Changes indicates the callback should only be called for records that
	// have changed.
	Changes bool
}

// A Value is a terminal record value.
type Value interface {
	Record

	// GetValue returns the value.
	GetValue() (value encoding.BinaryValue, version int, err error)

	// LoadValue stores the value of the reader into the receiver.
	LoadValue(value Value, put bool) error

	// LoadBytes unmarshals a value from bytes into the receiver.
	LoadBytes(data []byte, put bool) error
}

// A Store loads and stores values.
type Store interface {
	// GetValue loads the value from the underlying store and writes it. Byte
	// stores call LoadBytes(data) and value stores call LoadValue(v, false).
	GetValue(key *record.Key, value Value) error

	// PutValue gets the value from the reader and stores it. A byte store
	// marshals the value and stores the bytes. A value store finds the
	// appropriate value and calls LoadValue(v, true).
	PutValue(key *record.Key, value Value) error
}
