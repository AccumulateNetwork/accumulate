package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

//go:generate go run ../tools/cmd/gen-types --language go-union --out unions_gen.go accounts.yml general.yml internal.yml query.yml transactions.yml

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

type Account interface {
	encoding.BinaryValue
	GetType() AccountType
	Type() AccountType
	Header() *AccountHeader
	CopyAsInterface() interface{}
}

type SignerAccount interface {
	Account
	KeyHolder
	CreditHolder
	GetSignatureThreshold() uint64
	GetVersion() uint64
}

type TokenHolderAccount interface {
	Account
	TokenHolder
}

type Signature interface {
	encoding.BinaryValue
	Type() SignatureType
	Verify(hash []byte) bool
	Hash() []byte
	MetadataHash() []byte
	InitiatorHash() ([]byte, error)
	GetVote() VoteType

	GetSigner() *url.URL
	GetSignerVersion() uint64 // TODO Rename to GetSignerVersion
	GetTimestamp() uint64
	GetPublicKey() []byte
	GetSignature() []byte // TODO Remove once the API is improved
}

type TransactionBody interface {
	encoding.BinaryValue
	GetType() TransactionType
	Type() TransactionType
}

type TransactionResult interface {
	GetType() TransactionType
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
