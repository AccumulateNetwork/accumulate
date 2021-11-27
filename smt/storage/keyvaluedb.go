package storage

import (
	"errors"
)

// ErrNotFound is returned by KeyValueDB.Get if the key is not found
var ErrNotFound = errors.New("not found")

type KeyValueDB interface {
	Close() error                                // Returns an error if the close fails
	InitDB(filepath string, logger Logger) error // Sets up the database, returns error if it fails
	Get(key Key) (value []byte, err error)       // Get key from database, returns ErrNotFound if the key is not found
	Put(key Key, value []byte) error             // Put the value in the database, throws an error if fails
	EndBatch(map[Key][]byte) error               // End and commit a batch of transactions
}

// Logger defines a generic logging interface compatible with Tendermint (stolen from Tendermint).
type Logger interface {
	Debug(msg string, keyVals ...interface{})
	Info(msg string, keyVals ...interface{})
	Error(msg string, keyVals ...interface{})
}
