// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type LockAccount struct{}

func (LockAccount) Type() protocol.TransactionType { return protocol.TransactionTypeLockAccount }

func (LockAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return LockAccount{}.Validate(st, tx)
}

func (LockAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.LockAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.LockAccount), tx.Transaction.Body)
	}

	account, ok := st.Origin.(protocol.LockableAccount)
	if !ok {
		return nil, errors.BadRequest.WithFormat("locking is not supported for %v accounts", st.Origin.Type())
	}

	err := account.SetLockHeight(body.Height)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("set lock height: %w", err)
	}

	err = st.Update(account)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store account state: %w", err)
	}

	return nil, nil
}
