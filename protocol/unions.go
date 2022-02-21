package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
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

type Account interface {
	encoding.BinaryValue
	GetType() AccountType
	Type() AccountType
	Header() *AccountHeader
}

type Signature interface {
	encoding.BinaryValue
	Type() SignatureType

	GetPublicKey() []byte
	GetSignature() []byte

	Sign(nonce uint64, privateKey []byte, msghash []byte) error
	Verify(hash []byte) bool
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
