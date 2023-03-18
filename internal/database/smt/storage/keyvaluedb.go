// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package storage

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ErrNotFound is returned by KeyValueDB.Get if the key is not found.
var ErrNotFound = errors.NotFound

// ErrNotOpen is returned by KeyValueDB.Get, .Put, and .Close if the database is
// not open.
var ErrNotOpen = errors.InternalError.With("not open")

type Beginner interface {
	// Begin begins a transaction or sub-transaction.
	Begin(writable bool) KeyValueTxn

	BeginWithPrefix(writable bool, prefix string) KeyValueTxn
}

type KeyValueTxn interface {
	Beginner

	// Get gets a value.
	Get(key Key) ([]byte, error)
	// Put puts a value.
	Put(key Key, value []byte) error
	// PutAll puts many values.
	PutAll(map[Key][]byte) error
	// Commit commits the transaction.
	Commit() error
	// Discard discards the transaction.
	Discard()
}

type KeyValueStore interface {
	Beginner

	// Close closes the store.
	Close() error
}

// Logger defines a generic logging interface compatible with Tendermint (stolen from Tendermint).
type Logger interface {
	Debug(msg string, keyVals ...interface{})
	Info(msg string, keyVals ...interface{})
	Error(msg string, keyVals ...interface{})
}
