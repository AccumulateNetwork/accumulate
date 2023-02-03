// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticDepositTokens struct{}

var _ PrincipalValidator = (*SyntheticDepositTokens)(nil)
var _ TransactionExecutorCleanup = (*SyntheticDepositTokens)(nil)

func (SyntheticDepositTokens) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositTokens
}

func (SyntheticDepositTokens) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// SyntheticDepositTokens can create a lite token account
	key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	return key != nil
}

func (SyntheticDepositTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticDepositTokens{}).Validate(st, tx)
}

func (SyntheticDepositTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticDepositTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositTokens), tx.Transaction.Body)
	}

	var account protocol.AccountWithTokens
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteTokenAccount:
			account = origin
		case *protocol.TokenAccount:
			account = origin
		default:
			return nil, fmt.Errorf("invalid principal: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.Type())
		}
		if !account.GetTokenUrl().Equal(body.Token) {
			return nil, fmt.Errorf("token type mismatch: want %s, got %s", account.GetTokenUrl(), body.Token)
		}
	} else if keyHash, tok, err := protocol.ParseLiteTokenAddress(tx.Transaction.Header.Principal); err != nil {
		return nil, fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return nil, errors.NotFound.WithFormat("could not find token account")
	} else if !body.Token.Equal(tok) {
		return nil, fmt.Errorf("token URL does not match lite token account URL")
	} else {
		// Check the account limit
		liteIdUrl := tx.Transaction.Header.Principal.RootIdentity()
		dir, err := st.batch.Account(liteIdUrl).Directory().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load directory index: %w", err)
		}

		if len(dir)+1 > int(st.Globals.Globals.Limits.IdentityAccounts) {
			return nil, errors.BadRequest.WithFormat("identity would have too many accounts")
		}

		// Address is lite and the account doesn't exist, so create one
		lite := new(protocol.LiteTokenAccount)
		lite.Url = tx.Transaction.Header.Principal
		lite.TokenUrl = body.Token
		account = lite

		var liteIdentity *protocol.LiteIdentity
		err = st.LoadUrlAs(liteIdUrl, &liteIdentity)
		switch {
		case err == nil:
			// OK
		case errors.Is(err, storage.ErrNotFound):
			liteIdentity = new(protocol.LiteIdentity)
			liteIdentity.Url = liteIdUrl
			err := st.Update(liteIdentity)
			if err != nil {
				return nil, fmt.Errorf("failed to update %v: %v", liteIdentity.GetUrl(), err)
			}
		default:
			return nil, err
		}

		err = st.AddDirectoryEntry(liteIdUrl, tx.Transaction.Header.Principal)
		if err != nil {
			return nil, fmt.Errorf("failed to add directory entries in lite token account %s: %v", tx.Transaction.Header.Principal.RootIdentity(), err)
		}
	}

	if body.Amount.Sign() < 0 {
		return nil, fmt.Errorf("amount can't be a negative value")
	}

	if !account.CreditTokens(&body.Amount) {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}
	err := st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", account.GetUrl(), err)
	}
	return nil, nil
}

func (SyntheticDepositTokens) DidFail(state *ProcessTransactionState, transaction *protocol.Transaction) error {
	body, ok := transaction.Body.(*protocol.SyntheticDepositTokens)
	if !ok {
		return fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositTokens), transaction.Body)
	}

	// Send the tokens back on failure
	if !body.IsRefund {
		if body.IsIssuer {
			refund := new(protocol.SyntheticBurnTokens)
			refund.Amount = body.Amount
			refund.IsRefund = true
			state.DidProduceTxn(body.Cause.Account(), refund)
		} else {
			refund := new(protocol.SyntheticDepositTokens)
			refund.Token = body.Token
			refund.Amount = body.Amount
			refund.IsRefund = true
			state.DidProduceTxn(body.Cause.Account(), refund)
		}
	}

	return nil
}
