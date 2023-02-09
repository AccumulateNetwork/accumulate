// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionContext is the context in which a transaction is executed.
type TransactionContext struct {
	*MessageContext
	transaction *protocol.Transaction
}

func (x *TransactionContext) Type() protocol.TransactionType { return x.transaction.Body.Type() }

func (x *TransactionContext) validate(batch *database.Batch, status *protocol.TransactionStatus, principal protocol.Account) (protocol.TransactionResult, error) {
	// New executor
	if exec, ok := x.Executor.transactionExecutors[x.transaction.Body.Type()]; ok {
		// Process the transaction
		st, err := exec.Process(batch, x)
		err = errors.UnknownError.Wrap(err)
		if st == nil || st.Result == nil {
			return new(protocol.EmptyResult), nil
		}
		return st.Result, err
	}

	// Old executor
	if exec, ok := x.Executor.executors[x.transaction.Body.Type()]; ok {
		// Set up the state manager
		st := chain.NewStateManager(&x.Executor.Describe, &x.Executor.globals.Active, batch.Begin(false), principal, x.transaction, x.Executor.logger.With("operation", "ValidateEnvelope"))
		defer st.Discard()
		st.Pretend = true

		delivery := &chain.Delivery{Transaction: x.transaction}
		delivery.Internal = x.isWithin(internal.MessageTypeNetworkUpdate)
		result, err := exec.Validate(st, delivery)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		return result, nil
	}

	return nil, errors.InternalError.WithFormat("missing executor for %v", x.transaction.Body.Type())
}

func (x *TransactionContext) callTransactionExecutor(batch *database.Batch, status *protocol.TransactionStatus, principal protocol.Account) (protocol.TransactionResult, *chain.ProcessTransactionState, error) {
	// New executor
	if exec, ok := x.Executor.transactionExecutors[x.transaction.Body.Type()]; ok {
		// Process the transaction
		st, err := exec.Process(batch, x)
		err = errors.UnknownError.Wrap(err)
		if st == nil || st.Result == nil {
			return new(protocol.EmptyResult), new(chain.ProcessTransactionState), nil
		}
		return st.Result, new(chain.ProcessTransactionState), err
	}

	// Old executor
	if exec, ok := x.Executor.executors[x.transaction.Body.Type()]; ok {
		// Set up the state manager
		st := chain.NewStateManager(&x.Executor.Describe, &x.Executor.globals.Active, batch.Begin(true), principal, x.transaction, x.Executor.logger.With("operation", "ProcessTransaction"))
		defer st.Discard()

		r2 := x.Executor.BlockTimers.Start(exec.Type())
		delivery := &chain.Delivery{Transaction: x.transaction}
		delivery.Internal = x.isWithin(internal.MessageTypeNetworkUpdate)
		result, err := exec.Execute(st, delivery)
		x.Executor.BlockTimers.Stop(r2)
		if err != nil {
			err = errors.UnknownError.Wrap(err)
			return nil, nil, errors.UnknownError.Wrap(err)
		}

		// Do extra processing for special network accounts
		err = x.Executor.processNetworkAccountUpdates(st.GetBatch(), delivery, principal)
		if err != nil {
			return nil, nil, errors.UnknownError.Wrap(err)
		}

		// Commit changes, queue state creates for synthetic transactions
		state, err := st.Commit()
		if err != nil {
			err = fmt.Errorf("commit: %w", err)
			return nil, nil, errors.UnknownError.Wrap(err)
		}

		return result, state, nil
	}

	return nil, nil, errors.InternalError.WithFormat("missing executor for %v", x.transaction.Body.Type())
}
