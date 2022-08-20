package storage

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ErrNotFound is returned by KeyValueDB.Get if the key is not found.
var ErrNotFound = errors.StatusNotFound

// ErrNotOpen is returned by KeyValueDB.Get, .Put, and .Close if the database is
// not open.
var ErrNotOpen = errors.New(errors.StatusInternalError, "not open")

type KeyValueTxn interface {
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
	// Begin begins a sub-transaction.
	Begin(writable bool) KeyValueTxn
}

type KeyValueStore interface {
	// Close closes the store.
	Close() error
	// Begin begins a transaction.
	Begin(writable bool) KeyValueTxn
}

// Logger defines a generic logging interface compatible with Tendermint (stolen from Tendermint).
type Logger interface {
	Debug(msg string, keyVals ...interface{})
	Info(msg string, keyVals ...interface{})
	Error(msg string, keyVals ...interface{})
}
