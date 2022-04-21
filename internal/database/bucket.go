package database

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

const (
	// Top level buckets
	kAccount     = "Account"
	kTransaction = "Transaction"

	// Account/transaction buckets
	kState = "State"
	kIndex = "Index"
	kChain = "Chain"
	kData  = "Data"

	// State
	kPrimary   = "Primary"
	kSecondary = "Secondary"
	kSignature = "Signature"
	kSynthetic = "Synthetic"
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

// // State returns the storage key for the object's state.
// func (b *objectBucket) State(name string) storage.Key {
// 	return b.Object().Append(kState, name)
// }

// Index returns the storage key for the given index of the object.
func (b *objectBucket) Index(key ...interface{}) storage.Key {
	return b.Object().Append(kIndex).Append(key...)
}

// Data returns the storage key for the given data entry of the object.
func (b *objectBucket) Data(key ...interface{}) storage.Key {
	return b.Object().Append(kData).Append(key...)
}

// accountBucket is a database bucket for a account.
type accountBucket struct{ objectBucket }

// account returns a accountBucket for the account with the given URL.
func account(u *url.URL) accountBucket {
	return accountBucket{object(kAccount, u)}
}

// accountByID returns a accountBucket for the account with the given ID.
func accountByID(id []byte) accountBucket {
	return accountBucket{object(kAccount, id)}
}

func (b *accountBucket) PrimaryState() storage.Key {
	return b.Object().Append(kState, kPrimary)
}

func (b *accountBucket) SyntheticForAnchor(anchor [32]byte) storage.Key {
	return b.Object().Append(kState, kSynthetic, anchor)
}

// Chain returns the storage key for the given chain of the record.
func (b *accountBucket) Chain(name string) storage.Key {
	// Ensure chain names are case insensitive
	name = strings.ToLower(name)
	return b.Object().Append(kChain, name)
}

// transactionBucket is a database bucket for a transaction.
type transactionBucket struct{ objectBucket }

// transaction returns a transactionBucket for the transaction with the given ID.
func transaction(txid []byte) transactionBucket {
	return transactionBucket{object(kTransaction, txid)}
}

func (b *transactionBucket) PrimaryState() storage.Key {
	return b.Object().Append(kState, kPrimary)
}

// SecondaryState returns the storage key for the transaction's status.
func (b *transactionBucket) SecondaryState() storage.Key {
	return b.Object().Append(kState, kSecondary)
}

// Signatures returns the storage key for the transaction's signatures.
func (b *transactionBucket) Signatures(signer *url.URL) storage.Key {
	// TODO Handle this through signature chains.
	return b.Object().Append(kState, kSignature, signer)
}

// Synthetic returns the storage key for IDs of synthetic transactions produced
// by the transaction.
func (b *transactionBucket) Synthetic() storage.Key {
	return b.Object().Append(kState, kSynthetic)
}
