package storage

const (
	KeyLength = 32 // Total bytes used for keys
)

type KeyValueDB interface {
	Close() error                                // Returns an error if the close fails
	InitDB(filepath string) error                // Sets up the database, returns error if it fails
	Get(key [KeyLength]byte) (value []byte)      // Get key from database, on not found, error returns nil
	Put(key [KeyLength]byte, value []byte) error // Put the value in the database, throws an error if fails
	EndBatch(map[[KeyLength]byte][]byte) error   // End and commit a batch of transactions
}
