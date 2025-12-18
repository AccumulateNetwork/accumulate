// Copyright 2025 The Accumulate Authors
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

type SetLiteAccountDelegate struct{}

func (SetLiteAccountDelegate) Type() protocol.TransactionType {
	return protocol.TransactionTypeSetLiteAccountDelegate
}

func (x SetLiteAccountDelegate) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (SetLiteAccountDelegate) check(st *StateManager, tx *Delivery) (*protocol.SetLiteAccountDelegate, error) {
	body, ok := tx.Transaction.Body.(*protocol.SetLiteAccountDelegate)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SetLiteAccountDelegate), tx.Transaction.Body)
	}

	return body, nil
}

func (x SetLiteAccountDelegate) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Verify principal is a lite token account
	if _, ok := st.Origin.(*protocol.LiteTokenAccount); !ok {
		return nil, errors.BadRequest.WithFormat(
			"SetLiteAccountDelegate requires a lite token account, got %v",
			st.Origin.Type())
	}

	// If setting a delegate, verify target is a valid authority (KeyBook) if it exists locally
	if body.Delegate != nil {
		delegate, err := st.batch.Account(body.Delegate).Main().Get()
		switch {
		case err == nil:
			// Account exists locally - verify it's a KeyBook
			switch delegate.(type) {
			case *protocol.KeyBook:
				// Valid delegate target (per issue spec, LiteIdentity is not allowed as delegate)
			default:
				return nil, errors.BadRequest.WithFormat(
					"delegate must be a KeyBook, got %v",
					delegate.Type())
			}
		case errors.Is(err, errors.NotFound):
			// Account not found locally - could be remote, allow for now
			// TODO: For remote delegates, require a proof of existence (similar to UpdateAccountAuth)
		default:
			return nil, errors.UnknownError.WithFormat("load delegate: %w", err)
		}
	}

	// Update the account's delegate field
	st.Origin.(*protocol.LiteTokenAccount).Delegate = body.Delegate

	err = st.Update(st.Origin)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store account state: %w", err)
	}

	return nil, nil
}
