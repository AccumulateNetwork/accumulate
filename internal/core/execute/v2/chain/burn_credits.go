// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type BurnCredits struct{}

func (BurnCredits) Type() protocol.TransactionType { return protocol.TransactionTypeBurnCredits }

func (x BurnCredits) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (BurnCredits) check(st *StateManager, tx *Delivery) (*protocol.BurnCredits, error) {
	body, ok := tx.Transaction.Body.(*protocol.BurnCredits)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %v, got %v", protocol.TransactionTypeBurnCredits, tx.Transaction.Body.Type())
	}

	// BurnCredits has no fee so we must enforce a minimum burn amount
	if body.Amount < uint64(protocol.FeeMinimumCreditPurchase) {
		return nil, errors.BadRequest.WithFormat("invalid amount %v, minimum is %v",
			protocol.FormatAmount(body.Amount, protocol.CreditPrecisionPower),
			protocol.FormatAmount(uint64(protocol.FeeMinimumCreditPurchase), protocol.CreditPrecisionPower))
	}

	return body, nil
}

func (x BurnCredits) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Ensure the principal is a credit account
	account, err := loadCreditAccount(st.batch, st.OriginUrl, "invalid principal")
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if !account.DebitCredits(body.Amount) {
		return nil, errors.BadRequest.WithFormat("insufficient balance %v, attempted to burn %v",
			protocol.FormatAmount(account.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(body.Amount, protocol.CreditPrecisionPower))
	}

	err = st.Update(account)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store account: %w", err)
	}

	return nil, nil
}
