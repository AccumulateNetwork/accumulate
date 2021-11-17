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
func (b *Bucket) Put(key []byte, value []byte) {
	if b.ref == nil {
		b.ref = make(map[[32]byte]int)
	}
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

type DB interface {
	Close() error                                            // Returns an error if the close fails
	InitDB(filepath string) error                            // Sets up the database, returns error if it fails
	Get(bucket []byte, key []byte) (value []byte, err error) // Get key from database, returns ErrNotFound if the key is not found
	Put(bucket []byte, key []byte, value []byte) error       // Put the value in the database, throws an error if fails
	GetBucket(bucket []byte) (*Bucket, error)                // GetBucket retrieves all the data contained within a bucket
}
