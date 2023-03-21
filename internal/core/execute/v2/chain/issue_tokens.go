// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type IssueTokens struct{}

func (IssueTokens) Type() protocol.TransactionType { return protocol.TransactionTypeIssueTokens }

func (x IssueTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (IssueTokens) check(st *StateManager, tx *Delivery) ([]*protocol.TokenRecipient, error) {
	body, ok := tx.Transaction.Body.(*protocol.IssueTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.IssueTokens), tx.Transaction.Body)
	}

	// Normalize
	recipients := body.To
	if body.Recipient != nil {
		// Make a copy so we don't change the original transaction
		recipients = make([]*protocol.TokenRecipient, len(recipients)+1)
		recipients[0] = &protocol.TokenRecipient{
			Url:    body.Recipient,
			Amount: body.Amount,
		}
		copy(recipients[1:], body.To)
	}

	for _, to := range recipients {
		if to.Url == nil {
			return nil, errors.BadRequest.WithFormat("recipient URL is missing")
		}
		if to.Amount.Sign() < 0 {
			return nil, fmt.Errorf("amount can't be a negative value")
		}
	}

	return recipients, nil
}

func (x IssueTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	recipients, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	issuer, ok := st.Origin.(*protocol.TokenIssuer)
	if !ok {
		return nil, fmt.Errorf("invalid principal: want chain type %v, got %v", protocol.AccountTypeTokenIssuer, st.Origin.Type())
	}

	// Calculate the total and update Issued
	total := new(big.Int)
	for _, to := range recipients {
		total.Add(total, &to.Amount)
	}
	if !issuer.Issue(total) {
		return nil, fmt.Errorf("cannot exceed supply limit")
	}
	err = st.Update(issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", issuer.Url, err)
	}

	for _, to := range recipients {
		deposit := new(protocol.SyntheticDepositTokens)
		deposit.Token = issuer.Url
		deposit.Amount = to.Amount
		deposit.IsIssuer = true
		st.Submit(to.Url, deposit)
	}

	return nil, nil
}
