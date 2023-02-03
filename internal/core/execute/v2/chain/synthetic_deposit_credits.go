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

type SyntheticDepositCredits struct{}

var _ TransactionExecutorCleanup = (*SyntheticDepositCredits)(nil)

var _ PrincipalValidator = (*SyntheticDepositCredits)(nil)

func (SyntheticDepositCredits) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositCredits
}

func (SyntheticDepositCredits) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// The principal can be missing if it is a lite identity
	key, _ := protocol.ParseLiteIdentity(transaction.Header.Principal)
	return key != nil
}

func (SyntheticDepositCredits) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticDepositCredits{}).Validate(st, tx)
}

func (SyntheticDepositCredits) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticDepositCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositCredits), tx.Transaction.Body)
	}

	// Update the signer
	var account protocol.Signer
	var create bool
	switch origin := st.Origin.(type) {
	case nil:
		// Create a new lite identity
		create = true
		key, _ := protocol.ParseLiteIdentity(tx.Transaction.Header.Principal)
		if key == nil {
			return nil, errors.NotFound.WithFormat("%v not found", tx.Transaction.Header.Principal)
		}
		account = &protocol.LiteIdentity{Url: tx.Transaction.Header.Principal}

	case *protocol.LiteIdentity:
		account = origin

	case *protocol.KeyPage:
		account = origin

	default:
		return nil, fmt.Errorf("invalid principal: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	account.CreditCredits(body.Amount)

	// Update the ledger
	var ledgerState *protocol.SystemLedger
	err := st.LoadUrlAs(st.NodeUrl(protocol.Ledger), &ledgerState)
	if err != nil {
		return nil, err
	}

	if body.AcmeRefundAmount != nil {
		ledgerState.AcmeBurnt.Add(&ledgerState.AcmeBurnt, body.AcmeRefundAmount)
		err = st.Update(ledgerState)
		if err != nil {
			return nil, err
		}
	}

	// Persist the signer
	if create {
		err = st.Create(account)
	} else {
		err = st.Update(account)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
	}
	return nil, nil
}

func (SyntheticDepositCredits) DidFail(state *ProcessTransactionState, transaction *protocol.Transaction) error {
	body, ok := transaction.Body.(*protocol.SyntheticDepositCredits)
	if !ok {
		return fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositCredits), transaction.Body)
	}

	if body.AcmeRefundAmount != nil && !body.IsRefund {
		refund := new(protocol.SyntheticDepositTokens)
		refund.Token = protocol.AcmeUrl()
		refund.Amount = *body.AcmeRefundAmount
		refund.IsRefund = true
		state.DidProduceTxn(body.Source(), refund)
	}
	return nil
}
