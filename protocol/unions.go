package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

//go:generate go run ../tools/cmd/gen-types --language go-union --out unions_gen.go account_auth_operations.yml accounts.yml general.yml internal.yml key_page_operations.yml query.yml signatures.yml synthetic_transactions.yml transaction.yml transaction_results.yml user_transactions.yml

// AccountType is the type of an account.
type AccountType uint64

// SignatureType is the type of a transaction signature.
type SignatureType uint64

// TransactionType is the type for transaction types.
type TransactionType uint64

// TransactionMax defines the max point for transaction types.
type TransactionMax uint64

// VoteType specifies how the user wants to vote on a proposal (e.g. transaction, initiative, etc)
type VoteType uint64

type Signature interface {
	encoding.BinaryValue
	Type() SignatureType
	Verify(hash []byte) bool
	Hash() []byte
	MetadataHash() []byte
	InitiatorHash() ([]byte, error)
	GetVote() VoteType

	GetSigner() *url.URL
	GetSignerVersion() uint64
	GetTimestamp() uint64
	GetPublicKeyHash() []byte
	GetSignature() []byte
}

type TransactionBody interface {
	encoding.BinaryValue
	Type() TransactionType
}

type TransactionResult interface {
	Type() TransactionType
	encoding.BinaryValue
}

type KeyPageOperation interface {
	Type() KeyPageOperationType
	encoding.BinaryValue
}

type AccountAuthOperation interface {
	Type() AccountAuthOperationType
	encoding.BinaryValue
}
