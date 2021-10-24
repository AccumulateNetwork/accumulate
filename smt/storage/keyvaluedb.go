package storage

type KeyValueDB interface {
	Close() error                          // Returns an error if the close fails
	InitDB(filepath string) error          // Sets up the database, returns error if it fails
	Get(key Key) (value []byte, err error) // Get key from database, on not found, error returns nil
	Put(key Key, value []byte) error       // Put the value in the database, throws an error if fails
	EndBatch(map[Key][]byte) error         // End and commit a batch of transactions
}
