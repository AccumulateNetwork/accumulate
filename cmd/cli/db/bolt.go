package db

import (
	"fmt"

	"github.com/boltdb/bolt"
)

type BoltDB struct {
	db *bolt.DB
}

//Close the database
func (b *BoltDB) Close() error {
	if b.db == nil {
		return fmt.Errorf("database not open")
	}
	return b.db.Close()
}

//InitDB will open the database
func (b *BoltDB) InitDB(filename string) (err error) {
	b.db, err = bolt.Open(filename, 0600, nil)
	return err
}

//Get will get an entry in the database given a bucket and key
func (b *BoltDB) Get(bucket []byte, key []byte) (value []byte, err error) {
	if b.db == nil {
		return nil, fmt.Errorf("database not open")
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return fmt.Errorf("bucket not defined")
		}
		value = b.Get(key)
		if value == nil {
			return fmt.Errorf("key not found")
		}
		return err
	})

	return value, err
}

//Put will write data to a given bucket using the key
func (b *BoltDB) Put(bucket []byte, key []byte, value []byte) error {
	if b.db == nil {
		return fmt.Errorf("database not open")
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		buck, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return fmt.Errorf("DB: %s", err)
		}
		return buck.Put(key, value)
	})
}

//GetBucket will return the contents of a bucket
func (b *BoltDB) GetBucket(bucket []byte) (buck *Bucket, err error) {
	if b.db == nil {
		return nil, fmt.Errorf("database not open")
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return fmt.Errorf("bucket not defined")
		}
		c := b.Cursor()
		buck = new(Bucket)
		for k, v := c.First(); k != nil; k, v = c.Next() {
			buck.Put(k, v)
		}
		return err
	})

	return buck, err
}

// Delete will remove a key/value pair from the bucket
func (b *BoltDB) Delete(bucket []byte, key []byte) error {
	if b.db == nil {
		return fmt.Errorf("database not open")
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		buck := tx.Bucket(bucket)
		if buck != nil {
			return nil
		}
		return buck.Delete(key)
	})
}

// DeleteBucket will delete all key/value pairs from a bucket
func (b *BoltDB) DeleteBucket(bucket []byte) error {
	if b.db == nil {
		return fmt.Errorf("database not open")
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(bucket)
	})
}
