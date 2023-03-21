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

type BurnTokens struct{}

func (BurnTokens) Type() protocol.TransactionType { return protocol.TransactionTypeBurnTokens }

func (x BurnTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (BurnTokens) check(st *StateManager, tx *Delivery) (*protocol.BurnTokens, error) {
	body, ok := tx.Transaction.Body.(*protocol.BurnTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.BurnTokens), tx.Transaction.Body)
	}

	if body.Amount.Sign() < 0 {
		return nil, fmt.Errorf("amount can't be a negative value")
	}

	return body, nil
}

func (x BurnTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	var account protocol.AccountWithTokens
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin
	case *protocol.TokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid principal: want chain type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.Type())
	}

	if !account.DebitTokens(&body.Amount) {
		return nil, fmt.Errorf("cannot burn more tokens than is available in account")
	}

	burn := new(protocol.SyntheticBurnTokens)
	burn.Amount = body.Amount
	st.Submit(account.GetTokenUrl(), burn)

	err = st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
	}
	return nil, nil
}
