// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package db

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"

	"github.com/boltdb/bolt"
)

var ErrNotOpen = errors.New("database not open")
var ErrNotFound = errors.New("key not found")
var ErrNoBucket = errors.New("bucket not defined")
var ErrNoPassword = errors.New("no password specified for encrypted database")
var ErrInvalidPassword = errors.New("invalid password")
var ErrMalformedEncryptedDatabase = errors.New("malformed encrypted database")
var ErrDatabaseNotEncrypted = errors.New("database not encrypted")
var ErrDatabaseAlreadyEncrypted = errors.New("database already encrypted")

var BucketConfig = []byte("Config")

//WalletVersion is incremented whenever a bucket format is changed.
var WalletVersion = NewVersion(0, 0, 3, 0)

//magic is used for encryption verification when a database is opened
var magic = sha256.Sum256([]byte("accumulate"))

type BoltDB struct {
	db *bolt.DB

	filename string

	//salt is needed to generate the encryption key from the password
	salt []byte

	// encryptionkey is a hash of the password and salt
	encryptionkey []byte
}

func (b *BoltDB) Name() string {
	return b.filename
}

//Close the database
func (b *BoltDB) Close() error {
	if b.db == nil {
		return ErrNotOpen
	}
	return b.db.Close()
}

func (b *BoltDB) loadAndVerify(password string) error {
	//make sure the salt is clear
	b.salt = nil

	//now lets read the magic to see this is an encrypted database
	magicData, err := b.Get(BucketConfig, []byte("magic"))
	if err != nil {
		return ErrDatabaseNotEncrypted
	}

	//test to see if the magic is in the clear, if so then return the not encrypted error
	if bytes.Equal(magic[:], magicData) {
		return ErrDatabaseNotEncrypted
	}

	//unpack the salt
	b.salt, err = b.Get(BucketConfig, []byte("salt"))
	if err != nil {
		return err
	}

	b.encryptionkey, err = GetKey(password, b.salt)
	if err != nil {
		return err
	}

	//decrypt data using password
	data, err := Decrypt(magicData, b.encryptionkey)
	if err != nil {
		return ErrInvalidPassword
	}

	//now do a simple comparison of the decrypted data to what we expect to see
	if !bytes.Equal(data, magic[:]) {
		return ErrMalformedEncryptedDatabase
	}

	return nil
}

//InitDB will open the database
func (b *BoltDB) InitDB(filename string, password string) (err error) {
	b.db, err = bolt.Open(filename, 0600, nil)
	if err != nil {
		return ErrNotOpen
	}

	b.filename = filename

	//if a password is supplied, then check if it is encrypted.
	if password != "" {
		_, err := b.Get(BucketConfig, []byte("version"))
		if err != nil {
			//if we get here the database is new (or older and not encrypted), so let's get some salt
			salt := make([]byte, 30)
			_, err = rand.Read(salt)
			if err != nil {
				return err
			}

			key, err := GetKey(password, salt)
			if err != nil {
				return err
			}

			//store the encrypted magic code so we can do some quick decoding checks on startup
			data, err := Encrypt(magic[:], key)
			if err != nil {
				return err
			}
			err = b.PutRaw(BucketConfig, []byte("magic"), data)
			if err != nil {
				return err
			}

			//now store the salt
			err = b.Put(BucketConfig, []byte("salt"), salt)
			if err != nil {
				return err
			}

			err = b.PutRaw(BucketConfig, []byte("version"), WalletVersion.Bytes())
			if err != nil {
				return err
			}
		}
	}

	return b.loadAndVerify(password)
}

func (b *BoltDB) decryptIfNecessary(value []byte) ([]byte, error) {
	var err error
	//check to see if we need to decrypt the value
	if b.salt != nil {
		//if we have salt then the database is expected to be encrypted
		if b.encryptionkey == nil {
			return nil, ErrNoPassword
		}
		value, err = Decrypt(value, b.encryptionkey)
		if err != nil {
			return nil, ErrInvalidPassword
		}
	}
	return value, nil
}

func (b *BoltDB) encryptIfNecessary(value []byte) ([]byte, error) {
	var err error
	//check to see if we need to decrypt the value
	if b.salt != nil {
		//if we have salt then the database is expected to be encrypted
		if b.encryptionkey == nil {
			return nil, ErrNoPassword
		}
		value, err = Encrypt(value, b.encryptionkey)
		if err != nil {
			return nil, ErrInvalidPassword
		}
	}
	return value, nil
}

//Get will get an entry in the database given a bucket and key
func (b *BoltDB) GetRaw(bucket []byte, key []byte) (value []byte, err error) {
	if b.db == nil {
		return nil, ErrNotOpen
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(bucket)
		if bk == nil {
			return ErrNoBucket
		}
		value = bk.Get(key)
		if value == nil {
			return ErrNotFound
		}
		return nil
	})

	return value, err
}

//Get will get an entry in the database given a bucket and key
func (b *BoltDB) Get(bucket []byte, key []byte) (value []byte, err error) {
	value, err = b.GetRaw(bucket, key)
	if err != nil {
		return nil, err
	}
	return b.decryptIfNecessary(value)
}

func (b *BoltDB) PutRaw(bucket []byte, key []byte, value []byte) error {
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

//Put will write data to a given bucket using the key
func (b *BoltDB) Put(bucket []byte, key []byte, value []byte) error {
	value, err := b.encryptIfNecessary(value)
	if err != nil {
		return err
	}
	return b.PutRaw(bucket, key, value)
}

//GetBucket will return the contents of a bucket
func (b *BoltDB) GetBucket(bucket []byte) (buck *Bucket, err error) {
	if b.db == nil {
		return nil, ErrNotOpen
	}

	err = b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(bucket)
		if bk == nil {
			return ErrNoBucket
		}
		c := bk.Cursor()
		buck = new(Bucket)
		for k, v := c.First(); k != nil; k, v = c.Next() {
			v, err = b.decryptIfNecessary(v)
			if err != nil {
				return err
			}
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
