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

type SyntheticBurnTokens struct{}

func (SyntheticBurnTokens) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticBurnTokens
}

func (x SyntheticBurnTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (SyntheticBurnTokens) check(st *StateManager, tx *Delivery) (*protocol.SyntheticBurnTokens, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticBurnTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticBurnTokens), tx.Transaction.Body)
	}

	if body.Amount.Sign() < 0 {
		return nil, fmt.Errorf("amount can't be a negative value")
	}

	return body, nil
}

func (x SyntheticBurnTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	account, ok := st.Origin.(*protocol.TokenIssuer)
	if !ok {
		return nil, fmt.Errorf("invalid principal: want chain type %v, got %v", protocol.AccountTypeTokenIssuer, st.Origin.Type())
	}

	// Subtract the amount burnt from the amount issued
	account.Issued.Sub(&account.Issued, &body.Amount)

	// Verify that the result is not negative. The marshaller isn't capable of
	// encoding negative bigint values, so this would fail anyways
	if st.Globals.ExecutorVersion.V2VandenbergEnabled() && account.Issued.Sign() < 0 {
		return nil, errors.Conflict.With("acme burnt exceeds amount issued")
	}

	err = st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
	}
	return nil, nil
}
