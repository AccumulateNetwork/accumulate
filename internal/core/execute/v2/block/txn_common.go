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

type storeAccountFlags int

const (
	mustExist storeAccountFlags = 1 << iota
	mustNotExist
)

func (x *TransactionContext) storeAccount(batch *database.Batch, flag storeAccountFlags, accounts ...protocol.Account) error {
	for _, account := range accounts {
		// Check the path for invalid characters
		err := protocol.IsValidAccountPath(account.GetUrl().Path)
		if err != nil {
			return errors.BadRequest.WithFormat("invalid account path: %w", err)
		}

		// Check the path length
		if len(account.GetUrl().String()) > protocol.AccountUrlMaxLength {
			return errors.BadUrlLength.Wrap(fmt.Errorf("url specified exceeds maximum character length: %s", account.GetUrl().String()))
		}

		record := batch.Account(account.GetUrl())
		_, err = record.GetState()
		switch {
		case err == nil:
			if flag&mustNotExist != 0 {
				return errors.Conflict.WithFormat("account %v already exists", account.GetUrl())
			}
		case errors.Is(err, errors.NotFound):
			if flag&mustExist != 0 {
				return errors.Conflict.WithFormat("account %v already exists", account.GetUrl())
			}

			// Add it to the directory
			identity, ok := account.GetUrl().Parent()
			if !ok {
				break
			}
			err = batch.Account(identity).Directory().Add(account.GetUrl())
			if err != nil {
				return errors.UnknownError.WithFormat("add %v directory entry: %w", identity, err)
			}
		default:
			return errors.UnknownError.WithFormat("load account %v: %w", account.GetUrl(), err)
		}

		// if st.Pretend {
		// 	continue
		// }

		// Update/Create the state
		err = record.PutState(account)
		if err != nil {
			return errors.UnknownError.WithFormat("failed to update state of %q: %w", account.GetUrl(), err)
		}

		// Add to the account's main chain
		err = record.MainChain().Inner().AddHash(x.transaction.GetHash(), true)
		if err != nil {
			return errors.UnknownError.WithFormat("failed to update main chain of %q: %w", account.GetUrl(), err)
		}
	}

	return nil
}
