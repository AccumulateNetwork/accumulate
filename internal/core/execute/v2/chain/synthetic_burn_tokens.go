// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticBurnTokens struct{}

func (SyntheticBurnTokens) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticBurnTokens
}

func (SyntheticBurnTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticBurnTokens{}).Validate(st, tx)
}

func (SyntheticBurnTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticBurnTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticBurnTokens), tx.Transaction.Body)
	}

	account, ok := st.Origin.(*protocol.TokenIssuer)
	if !ok {
		return nil, fmt.Errorf("invalid principal: want chain type %v, got %v", protocol.AccountTypeTokenIssuer, st.Origin.Type())
	}

	if body.Amount.Sign() < 0 {
		return nil, fmt.Errorf("amount can't be a negative value")
	}

	account.Issued.Sub(&account.Issued, &body.Amount)

	err := st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
	}
	return nil, nil
}
