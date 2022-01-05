package database

import (
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// objectBucket is a database bucket for some object.
type objectBucket storage.Key

// object returns an objectBucket for the given key.
func object(key ...interface{}) objectBucket {
	return objectBucket(storage.MakeKey(key...))
}

// Object returns the storage key for the object itself.
func (b *objectBucket) Object() storage.Key {
	return storage.Key(*b)
}

// State returns the storage key for the object's state.
func (b *objectBucket) State() storage.Key {
	return b.Object().Append("State")
}

// Index returns the storage key for the given index of the object.
func (b *objectBucket) Index(key ...interface{}) storage.Key {
	return b.Object().Append("Index").Append(key...)
}

// Data returns the storage key for the given data entry of the object.
func (b *objectBucket) Data(key ...interface{}) storage.Key {
	return b.Object().Append("Data").Append(key...)
}

// recordBucket is a database bucket for a record.
type recordBucket struct{ objectBucket }

// record returns a recordBucket for the record with the given URL.
func record(u *url.URL) recordBucket {
	return recordBucket{object("Record", u)}
}

// record returns a recordBucket for the record with the given chain ID.
func recordFromChain(id []byte) recordBucket {
	return recordBucket{object("Record", id)}
}

// Chain returns the storage key for the given chain of the record.
func (b *recordBucket) Chain(name string) storage.Key {
	return b.Object().Append("Chain", name)
}

// recordBucket is a database bucket for a transaction.
type transactionBucket struct{ objectBucket }

// transaction returns a transactionBucket for the transaction with the given ID.
func transaction(txid []byte) transactionBucket {
	return transactionBucket{object("Transaction", txid)}
}

// Status returns the storage key for the transaction's status.
func (b *transactionBucket) Status() storage.Key {
	return b.Object().Append("Status")
}

// Signatures returns the storage key for the transaction's status.
func (b *transactionBucket) Signatures() storage.Key {
	return b.Object().Append("Signatures")
}

// Synthetic returns the storage key for IDs of synthetic transactions produced
// by the transaction.
func (b *transactionBucket) Synthetic() storage.Key {
	return b.Object().Append("Synthetic")
}
