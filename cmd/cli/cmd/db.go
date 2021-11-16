package cmd

import "errors"

// ErrNotFound is returned by KeyValueDB.Get if the key is not found
var ErrNotFound = errors.New("not found")

type KeyValue struct {
	Key   []byte
	Value []byte
}

type Bucket struct {
	KeyValue [][]byte
}

type WalletDB interface {
	Close() error                             // Returns an error if the close fails
	InitDB(filepath string) error             // Sets up the database, returns error if it fails
	Get(key []byte) (value []byte, err error) // Get key from database, returns ErrNotFound if the key is not found
	Put(key []byte, value []byte) error       // Put the value in the database, throws an error if fails

	GetBucket(bucket []byte, key []byte) (*Bucket, error)
}
