// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --language go-union --out unions_gen.go operations.yml accounts.yml general.yml system.yml key_page_operations.yml query.yml signatures.yml synthetic_transactions.yml transaction.yml transaction_results.yml user_transactions.yml

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

type TransactionBody interface {
	encoding.UnionValue
	Type() TransactionType
}

type TransactionResult interface {
	encoding.UnionValue
	Type() TransactionType
}

type KeyPageOperation interface {
	encoding.UnionValue
	Type() KeyPageOperationType
}

type AccountAuthOperation interface {
	encoding.UnionValue
	Type() AccountAuthOperationType
}

type NetworkMaintenanceOperation interface {
	encoding.UnionValue
	Type() NetworkMaintenanceOperationType
	ID() *url.TxID
}

func (p *PendingTransactionGCOperation) ID() *url.TxID {
	return p.Account.WithTxID(encoding.Hash(p))
}
