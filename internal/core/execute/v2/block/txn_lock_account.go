// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[LockAccount](&transactionExecutors, protocol.TransactionTypeLockAccount)
}

type LockAccount struct{}

func (LockAccount) Process(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	body, ok := ctx.transaction.Body.(*protocol.LockAccount)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid transaction: expected %v, got %v", protocol.TransactionTypeLockAccount, ctx.transaction.Body.Type())
	}

	principal, err := batch.Account(ctx.transaction.Header.Principal).Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load principal: %w", err)
	}

	account, ok := principal.(protocol.LockableAccount)
	if !ok {
		return nil, errors.BadRequest.WithFormat("locking is not supported for %v accounts", principal.Type())
	}

	err = account.SetLockHeight(body.Height)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("set lock height: %w", err)
	}

	err = batch.Account(ctx.transaction.Header.Principal).Main().Put(account)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store principal: %w", err)
	}

	return nil, nil
}
