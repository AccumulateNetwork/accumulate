// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package db

import "crypto/sha256"

//KeyValue holds the key/value data for the entry
type KeyValue struct {
	Key   []byte
	Value []byte
}

//Bucket is the structure to store the key/value data and a reference map to pull it.
type Bucket struct {
	KeyValueList []KeyValue       //KeyValueList contains the key/value data for the entry
	ref          map[[32]byte]int //ref maintains the map of the key to where the data is located in KeyValue List
}

//NewBucket creates a new instance of the bucket and initializes the map
func NewBucket() *Bucket {
	b := new(Bucket)
	b.ref = make(map[[32]byte]int)
	return b
}

//Put will store the key / value in the bucket
func (b *Bucket) Put(k []byte, v []byte) {
	if b.ref == nil {
		b.ref = make(map[[32]byte]int)
	}
	key := make([]byte, len(k))
	copy(key, k)
	value := make([]byte, len(v))
	copy(value, v)

	b.ref[sha256.Sum256(key)] = len(b.KeyValueList)

	b.KeyValueList = append(b.KeyValueList, KeyValue{key, value})
}

//Get will retrieve the value given a key
func (b *Bucket) Get(key []byte) (value []byte) {
	if b.ref == nil {
		return nil
	}

	if v, ok := b.ref[sha256.Sum256(key)]; ok {
		value = b.KeyValueList[v].Value
	}

	return value
}

//Delete will remove the value given a key
func (b *Bucket) Delete(key []byte) (err error) {
	kh := sha256.Sum256(key)
	if v, ok := b.ref[kh]; ok {
		var newList []KeyValue
		for i, k := range b.KeyValueList {
			if i == v {
				continue
			}
			newList = append(newList, k)
		}
		b.KeyValueList = newList
		delete(b.ref, kh)
	}
	return nil
}

//DB defines the interface functions to access the database
type DB interface {
	Close() error                                               // Returns an error if the close fails
	Name() string                                               // returns the database filename if applicable
	InitDB(filepath string, password string) error              // Sets up the database, returns error if it fails
	Get(bucket []byte, key []byte) (value []byte, err error)    // Get key from database (may further decrypt data if applicable), returns ErrNotFound if the key is not found
	Put(bucket []byte, key []byte, value []byte) error          // Put the value in the database (may further encrypt data if applicable), throws an error if fails
	GetRaw(bucket []byte, key []byte) (value []byte, err error) // GetRaw value as-is from database using key, returns ErrNotFound if the key is not found
	PutRaw(bucket []byte, key []byte, value []byte) error       // PutRaw the value in the database as-is, throws an error if fails
	GetBucket(bucket []byte) (*Bucket, error)                   // GetBucket retrieves all the data contained within a bucket
	Delete(bucket []byte, key []byte) error                     // Delete will remove a key/value pair from the bucket
	DeleteBucket(bucket []byte) error                           // DeleteBucket will delete all key/value pairs from a bucket
}
