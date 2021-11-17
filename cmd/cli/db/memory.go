package db

import (
	"crypto/sha256"
	"fmt"
)

type MemBucket struct {
	bucket Bucket
	ref    map[[32]byte]uint64
}

func NewMemBucket() *MemBucket {
	b := new(MemBucket)
	b.ref = make(map[[32]byte]uint64)
	return b
}

type MemoryDB struct {
	buckets map[[32]byte]*MemBucket
}

func (b *MemoryDB) Close() error {
	if b.buckets == nil {
		return fmt.Errorf("wallet database not open")
	}
	return nil
}

func (b *MemoryDB) InitDB(string) (err error) {
	b.buckets = make(map[[32]byte]*MemBucket)
	return err
}

func (b *MemoryDB) Get(bucket []byte, key []byte) (value []byte, err error) {
	if b.buckets == nil {
		return nil, fmt.Errorf("wallet database not open")
	}

	if v, ok := b.buckets[sha256.Sum256(bucket)]; ok {
		if idx, ok := v.ref[sha256.Sum256(key)]; ok {
			value = v.bucket.KeyValueList[idx].Value
		} else {
			err = fmt.Errorf("key not found")
		}
	} else {
		err = fmt.Errorf("bucket not defined")
	}
	return value, err
}

func (b *MemoryDB) Put(bucket []byte, key []byte, value []byte) error {
	if b.buckets == nil {
		return fmt.Errorf("wallet database not open")
	}

	var v *MemBucket
	var ok bool
	bk := sha256.Sum256(bucket)
	if v, ok = b.buckets[bk]; !ok {
		v = NewMemBucket()
		b.buckets[bk] = v
	}

	v.ref[sha256.Sum256(key)] = uint64(len(v.bucket.KeyValueList))
	v.bucket.KeyValueList = append(v.bucket.KeyValueList, KeyValue{key, value})
	return nil
}

func (b *MemoryDB) GetBucket(bucket []byte) (buck *Bucket, err error) {
	if b.buckets == nil {
		return nil, fmt.Errorf("wallet database not open")
	}

	if v, ok := b.buckets[sha256.Sum256(bucket)]; ok {
		buck = &v.bucket
	} else {
		err = fmt.Errorf("bucket not defined")
	}
	return buck, err
}
