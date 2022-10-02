// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package db

import (
	"crypto/sha256"
	"fmt"
)

//MemoryDB holds the main map of buckets for the in-memory database
type MemoryDB struct {
	buckets map[[32]byte]*Bucket
}

func (b *MemoryDB) Name() string {
	return "memory"
}

//Close clears out the data and uninitializes the MemoryDB
func (b *MemoryDB) Close() error {
	if b.buckets == nil {
		return fmt.Errorf("memory database not open")
	}
	b.buckets = nil
	return nil
}

//InitDB initializes the MemoryDB and must be called prior to use of the object
func (b *MemoryDB) InitDB(string, string) (err error) {
	b.buckets = make(map[[32]byte]*Bucket)
	return err
}

//Get will get an entry in the database given a bucket and key
func (b *MemoryDB) GetRaw(bucket []byte, key []byte) (value []byte, err error) {
	if b.buckets == nil {
		return nil, fmt.Errorf("memory database not initialized")
	}

	if v, ok := b.buckets[sha256.Sum256(bucket)]; ok {
		value = v.Get(key)
		if value == nil {
			err = ErrNotFound
		}
	} else {
		err = ErrNoBucket
	}
	return value, err
}
func (b *MemoryDB) Get(bucket []byte, key []byte) (value []byte, err error) {
	return b.GetRaw(bucket, key)
}

//PutRaw will write data to a given bucket using the key
func (b *MemoryDB) PutRaw(bucket []byte, key []byte, value []byte) error {
	if b.buckets == nil {
		return fmt.Errorf("memory database not initialized")
	}

	var v *Bucket
	var ok bool
	bk := sha256.Sum256(bucket)
	if v, ok = b.buckets[bk]; !ok {
		v = NewBucket()
		b.buckets[bk] = v
	}

	v.Put(key, value)
	return nil
}

//Put will write data to a given bucket using the key
func (b *MemoryDB) Put(bucket []byte, key []byte, value []byte) error {
	return b.PutRaw(bucket, key, value)
}

//GetBucket will return the contents of a bucket
func (b *MemoryDB) GetBucket(bucket []byte) (buck *Bucket, err error) {
	if b.buckets == nil {
		return nil, fmt.Errorf("memory database not initialized")
	}

	var ok bool
	if buck, ok = b.buckets[sha256.Sum256(bucket)]; !ok {
		err = ErrNoBucket
	}
	return buck, err
}

// Delete will remove a key/value pair from the bucket
func (b *MemoryDB) Delete(bucket []byte, key []byte) (err error) {
	if b.buckets == nil {
		return fmt.Errorf("memory database not initialized")
	}

	if buck, ok := b.buckets[sha256.Sum256(bucket)]; ok {
		err = buck.Delete(key)
	} else {
		err = ErrNoBucket
	}
	return err
}

// DeleteBucket will delete all key/value pairs from a bucket
func (b *MemoryDB) DeleteBucket(bucket []byte) error {
	delete(b.buckets, sha256.Sum256(bucket))
	return nil
}
