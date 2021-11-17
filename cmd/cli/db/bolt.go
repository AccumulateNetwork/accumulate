package db

import (
	"fmt"
	"github.com/boltdb/bolt"
)

type BoltDB struct {
	db *bolt.DB
}

func (b *BoltDB) Close() error {
	if b.db == nil {
		return fmt.Errorf("wallet database not open")
	}
	return b.db.Close()
}

func (b *BoltDB) InitDB(filename string) (err error) {
	b.db, err = bolt.Open(filename, 0600, nil)
	return err
}

func (b *BoltDB) Get(bucket []byte, key []byte) (value []byte, err error) {
	if b.db == nil {
		return nil, fmt.Errorf("wallet database not open")
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return fmt.Errorf("bucket not defined")
		}
		value = b.Get(key)
		return err
	})

	return value, err
}

func (b *BoltDB) Put(bucket []byte, key []byte, value []byte) error {
	if b.db == nil {
		return fmt.Errorf("wallet database not open")
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		buck, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return fmt.Errorf("DB: %s", err)
		}
		return buck.Put(key, value)
	})
}

func (b *BoltDB) GetBucket(bucket []byte) (buck *Bucket, err error) {
	if b.db == nil {
		return nil, fmt.Errorf("wallet database not open")
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return fmt.Errorf("bucket not defined")
		}
		c := b.Cursor()
		buck = new(Bucket)
		for k, v := c.First(); k != nil; k, v = c.Next() {
			buck.KeyValueList = append(buck.KeyValueList, KeyValue{k, v})
		}
		return err
	})

	return buck, err
}
