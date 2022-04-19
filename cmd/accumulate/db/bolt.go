package db

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"math/rand"
	"time"

	"github.com/boltdb/bolt"
)

var ErrNotOpen = errors.New("database not open")
var ErrNotFound = errors.New("key not found")
var ErrNoBucket = errors.New("bucket not defined")
var ErrNoPassword = errors.New("no password specified for encrypted database")
var ErrInvalidPassword = errors.New("invalid password")

var BucketSecureBucket = []byte("_secure_bucket")
var magic = sha256.Sum256([]byte("accumulate"))

type SecureDBMetaData struct {
	Salt      []byte
	Challenge []byte
}

func (m *SecureDBMetaData) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error unmarshalling: %v", r)
		}
	}()

	m.Salt, data = common.BytesSlice(data)
	m.Challenge, _ = common.BytesSlice(data)

	return err
}

func (m *SecureDBMetaData) MarshalBinary() ([]byte, error) {
	b := bytes.Buffer{}
	b.Write(common.SliceBytes(m.Salt))
	b.Write(common.SliceBytes(m.Challenge))
	return b.Bytes(), nil
}

type BoltDB struct {
	db *bolt.DB

	// Metadata includes salt
	metadata *SecureDBMetaData

	// encryptionkey is a hash of the password and salt
	encryptionkey []byte

	// Allow the wallet to be locked, by gating access based
	// on time.
	UnlockedUntil time.Time
}

//Close the database
func (b *BoltDB) Close() error {
	if b.db == nil {
		return ErrNotOpen
	}
	return b.db.Close()
}

//InitDB will open the database
func (b *BoltDB) InitDB(filename string, password string) (err error) {
	b.db, err = bolt.Open(filename, 0600, nil)
	if password != "" && err != nil {
		var data []byte
		data, err = b.Get(BucketSecureBucket, []byte("config"))
		b.metadata = new(SecureDBMetaData)
		if err != nil {
			salt := make([]byte, 30)
			_, err = rand.Read(salt)
			if err != nil {
				return err
			}

			b.metadata.Salt = salt
		} else {
			err = b.metadata.UnmarshalBinary(data)
			if err != nil {
				return err
			}

			data, err = b.Get(BucketSecureBucket, []byte("magic"))
			//TODO decrypt data using password
			if bytes.Compare(data, magic[:]) == 0 {
				return ErrInvalidPassword
			}
		}
	} else {
		_, err := b.Get(BucketSecureBucket, []byte("magic"))
		if err == nil {
			return ErrNoPassword
		}
	}

	return err
}

//Get will get an entry in the database given a bucket and key
func (b *BoltDB) Get(bucket []byte, key []byte) (value []byte, err error) {
	if b.db == nil {
		return nil, ErrNotOpen
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return ErrNoBucket
		}
		value = b.Get(key)
		if value == nil {
			return ErrNotFound
		}
		return err
	})

	return value, err
}

//Put will write data to a given bucket using the key
func (b *BoltDB) Put(bucket []byte, key []byte, value []byte) error {
	if b.db == nil {
		return ErrNotOpen
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
		return nil, ErrNotOpen
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return ErrNoBucket
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
		return ErrNotOpen
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		buck := tx.Bucket(bucket)
		if buck == nil {
			return ErrNoBucket
		}
		return buck.Delete(key)
	})
}

// DeleteBucket will delete all key/value pairs from a bucket
func (b *BoltDB) DeleteBucket(bucket []byte) error {
	if b.db == nil {
		return ErrNotOpen
	}

	return b.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(bucket)
	})
}
