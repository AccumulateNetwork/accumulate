package database

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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

// accountBucket is a database bucket for a account.
type accountBucket struct{ objectBucket }

// account returns a accountBucket for the account with the given URL.
func account(u *url.URL) accountBucket {
	return accountBucket{object("Account", u)}
}

// accountByID returns a accountBucket for the account with the given ID.
func accountByID(id []byte) accountBucket {
	return accountBucket{object("Account", id)}
}

func (b *accountBucket) SyntheticForAnchor(anchor [32]byte) storage.Key {
	return b.Object().Append("State", "Synthetic", anchor)
}

// Chain returns the storage key for the given chain of the record.
func (b *accountBucket) Chain(name string) record.Key {
	// Ensure chain names are case insensitive
	name = strings.ToLower(name)
	return record.Key{b.Object()}.Append("Chain", name)
}

// transactionBucket is a database bucket for a transaction.
type transactionBucket struct{ objectBucket }

// transaction returns a transactionBucket for the transaction with the given ID.
func transaction(txid []byte) transactionBucket {
	return transactionBucket{object("Transaction", txid)}
}

// Status returns the storage key for the transaction's status.
func (b *transactionBucket) Status() storage.Key {
	return b.Object().Append("Status")
}

// Signatures returns the storage key for the transaction's signatures.
func (b *transactionBucket) Signatures(signer *url.URL) storage.Key {
	// TODO Handle this through signature chains.
	return b.Object().Append("Signatures", signer)
}

// Synthetic returns the storage key for IDs of synthetic transactions produced
// by the transaction.
func (b *transactionBucket) Synthetic() storage.Key {
	return b.Object().Append("Synthetic")
}
