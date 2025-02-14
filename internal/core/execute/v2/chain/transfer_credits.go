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

type TransferCredits struct{}

func (TransferCredits) Type() protocol.TransactionType {
	return protocol.TransactionTypeTransferCredits
}

func (x TransferCredits) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (TransferCredits) check(st *StateManager, tx *Delivery) (*protocol.TransferCredits, error) {
	body, ok := tx.Transaction.Body.(*protocol.TransferCredits)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %v, got %v", protocol.TransactionTypeTransferCredits, tx.Transaction.Body.Type())
	}

	for _, to := range body.To {
		if to.Amount == 0 {
			return nil, errors.BadRequest.WithFormat("transfer amount must be non-zero")
		}
		if !to.Url.LocalTo(st.OriginUrl) {
			return nil, errors.BadRequest.WithFormat("cannot transfer credits outside of %v", st.OriginUrl.RootIdentity())
		}
	}

	return body, nil
}

func (x TransferCredits) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	var total uint64
	for _, to := range body.To {
		total += to.Amount
	}

	// Ensure the principal is a credit account
	account, err := loadCreditAccount(st.batch, st.OriginUrl, "invalid principal")
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if !account.DebitCredits(total) {
		return nil, errors.BadRequest.WithFormat("insufficient balance %v, attempted to transfer %v",
			protocol.FormatAmount(account.GetCreditBalance(), protocol.CreditPrecisionPower),
			protocol.FormatAmount(total, protocol.CreditPrecisionPower))
	}

	accounts := make([]protocol.Account, len(body.To)+1)
	accounts[0] = account

	for i, to := range body.To {
		account, err := loadCreditAccount(st.batch, to.Url, "invalid recipient")
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		accounts[i+1] = account
		account.CreditCredits(to.Amount)
	}

	err = st.Update(accounts...)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store account: %w", err)
	}

	return nil, nil
}
