package storage

import (
	"errors"
)

// ErrNotFound is returned by KeyValueDB.Get if the key is not found.
var ErrNotFound = errors.New("not found")

// ErrNotOpen is returned by KeyValueDB.Get, .Put, and .Close if the database is
// not open.
var ErrNotOpen = errors.New("not open")

type KeyValueTxn interface {
	Get(key Key) ([]byte, error)
	Put(key Key, value []byte) error
	PutAll(map[Key][]byte) error
	Commit() error
	Discard()
}

type KeyValueStore interface {
	Close() error                                // Returns an error if the close fails
	InitDB(filepath string, logger Logger) error // Sets up the database, returns error if it fails
	Begin(writable bool) KeyValueTxn
}

// Logger defines a generic logging interface compatible with Tendermint (stolen from Tendermint).
type Logger interface {
	Debug(msg string, keyVals ...interface{})
	Info(msg string, keyVals ...interface{})
	Error(msg string, keyVals ...interface{})
}
