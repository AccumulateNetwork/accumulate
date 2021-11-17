package db

type KeyValue struct {
	Key   []byte
	Value []byte
}

type Bucket struct {
	KeyValueList []KeyValue
}

type DB interface {
	Close() error                                            // Returns an error if the close fails
	InitDB(filepath string) error                            // Sets up the database, returns error if it fails
	Get(bucket []byte, key []byte) (value []byte, err error) // Get key from database, returns ErrNotFound if the key is not found
	Put(bucket []byte, key []byte, value []byte) error       // Put the value in the database, throws an error if fails

	GetBucket(bucket []byte) (*Bucket, error)
}
