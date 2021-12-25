package database

import (
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type objectBucket storage.Key

func object(key ...interface{}) objectBucket {
	return objectBucket(storage.MakeKey(key...))
}

func (b *objectBucket) Object() storage.Key {
	return storage.Key(*b)
}

func (b *objectBucket) State() storage.Key {
	return b.Object().Append("State")
}

func (b *objectBucket) Index(key ...interface{}) storage.Key {
	return b.Object().Append("Index").Append(key...)
}

func (b *objectBucket) Data(key ...interface{}) storage.Key {
	return b.Object().Append("Data").Append(key...)
}

type recordBucket struct{ objectBucket }

func record(u *url.URL) recordBucket {
	return recordBucket{object("Record", u)}
}

func recordFromChain(id []byte) recordBucket {
	return recordBucket{object("Record", id)}
}

func (b *recordBucket) Chain(name string) storage.Key {
	return b.Object().Append("Chain", name)
}

type transactionBucket struct{ objectBucket }

func transaction(txid []byte) transactionBucket {
	return transactionBucket{object("Transaction", txid)}
}

func (b *transactionBucket) Status() storage.Key {
	return b.Object().Append("Status")
}

func (b *transactionBucket) Signatures() storage.Key {
	return b.Object().Append("Signatures")
}

func (b *transactionBucket) Synthetic() storage.Key {
	return b.Object().Append("Synthetic")
}
