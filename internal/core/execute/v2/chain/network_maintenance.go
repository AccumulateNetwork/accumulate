// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkMaintenance struct{}

func (NetworkMaintenance) Type() protocol.TransactionType {
	return protocol.TransactionTypeNetworkMaintenance
}

func (x NetworkMaintenance) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	// If the authority set is not exactly what we expect, fall back
	ok, err := x.allowDnOperators(batch, transaction)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	if !ok {
		return true, nil
	}

	// dn.acme/operators is authorized to sign for partitions
	if protocol.DnUrl().JoinPath(protocol.Operators).Equal(sig.Authority) {
		return false, nil
	}

	return true, nil
}

func (x NetworkMaintenance) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	// If the authority set is not exactly what we expect, fall back
	ok, err := x.allowDnOperators(batch, transaction)
	if err != nil {
		return false, false, errors.UnknownError.Wrap(err)
	}
	if !ok {
		return false, true, nil
	}

	// The transaction is ready once the DN has voted
	ok, vote, err := delegate.AuthorityDidVote(batch, transaction, protocol.DnUrl().JoinPath(protocol.Operators))
	return ok && vote == protocol.VoteTypeAccept, false, err
}

func (NetworkMaintenance) allowDnOperators(batch *database.Batch, transaction *protocol.Transaction) (bool, error) {
	// Verify the principal is a partition ADI
	account, err := batch.Account(transaction.Header.Principal).Main().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load principal: %w", err)
	}
	adi, ok := account.(*protocol.ADI)
	if !ok {
		return false, errors.UnknownError.WithFormat("invalid principal: want %v, got %v", protocol.AccountTypeIdentity, account.Type())
	}

	// If the authority set is not exactly what we expect, fall back
	if len(adi.Authorities) != 1 || adi.Authorities[0].Disabled {
		return false, nil
	}
	book := adi.Authorities[0].Url
	if _, ok := protocol.ParsePartitionUrl(book); !ok ||
		!book.RootIdentity().Equal(adi.Url) ||
		!book.PathEqual(protocol.Operators) {
		return false, nil
	}

	return true, nil
}

func (x NetworkMaintenance) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (NetworkMaintenance) check(st *StateManager, tx *Delivery) (*protocol.NetworkMaintenance, error) {
	// Not enabled until Baikonur
	if !st.Globals.ExecutorVersion.V2BaikonurEnabled() {
		return nil, errors.BadRequest.WithFormat("unknown transaction type %v", tx.Transaction.Body.Type())
	}

	// Verify the body
	body, ok := tx.Transaction.Body.(*protocol.NetworkMaintenance)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid payload: want %v, got %v", protocol.TransactionTypeNetworkMaintenance, tx.Transaction.Body.Type())
	}

	// Verify the principal
	if _, ok := protocol.ParsePartitionUrl(st.OriginUrl); !ok ||
		!st.OriginUrl.PathEqual("") {
		return nil, errors.BadRequest.WithFormat("invalid principal: want partition, got %v", st.OriginUrl)
	}

	return body, nil
}

func (x NetworkMaintenance) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Verify the principal
	if !st.NodeUrl().Equal(st.OriginUrl) {
		return nil, errors.BadRequest.WithFormat("invalid principal: want %v, got %v", st.NodeUrl(), st.OriginUrl)
	}

	// For each operation
	for _, op := range body.Operations {
		// Verify the operation is valid
		switch op.(type) {
		case *protocol.PendingTransactionGCOperation:
			// Ok
		default:
			return nil, errors.UnknownError.Wrap(err)
		}

		// Queue it for execution. Operations that affect different domains
		// (root ADIs) can be executed concurrently. However they *cannot* be
		// executed concurrently with other types of operations (such as user
		// transactions) that target the same domain. Queueing each operation as
		// a new (internal) message allows the message processing system to
		// safely run unrelated operations concurrently (though that has not
		// been implemented yet).
		st.State.ProcessNetworkMaintenanceOp(tx.Transaction.ID(), op)
	}

	return nil, nil
}
