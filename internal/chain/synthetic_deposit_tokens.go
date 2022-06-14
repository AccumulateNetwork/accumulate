package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type SyntheticDepositTokens struct{}

var _ TransactionExecutorCleanup = (*SyntheticDepositTokens)(nil)

func (SyntheticDepositTokens) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositTokens
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
			tokenurl := origin.TokenUrl.String()
			if tokenurl != body.Token.String() {
				return nil, fmt.Errorf("token type mismatch want %s got %s", body.Token, tokenurl)
			}
		case *protocol.TokenAccount:
			account = origin
			tokenurl := origin.TokenUrl
			if tokenurl.String() != body.Token.String() {
				return nil, fmt.Errorf("token type mismatch want %s got %s", body.Token, tokenurl)
			}
		default:
			return nil, fmt.Errorf("invalid principal: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.Type())
		}
	} else if keyHash, tok, err := protocol.ParseLiteTokenAddress(tx.Transaction.Header.Principal); err != nil {
		return nil, fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return nil, errors.NotFound("could not find token account")
	} else if !body.Token.Equal(tok) {
		return nil, fmt.Errorf("token URL does not match lite token account URL")
	} else {
		// Address is lite and the account doesn't exist, so create one
		lite := new(protocol.LiteTokenAccount)
		lite.Url = tx.Transaction.Header.Principal
		lite.TokenUrl = body.Token
		account = lite

		liteIdUrl := tx.Transaction.Header.Principal.RootIdentity()
		var liteIdentity *protocol.LiteIdentity
		err := st.LoadUrlAs(liteIdUrl, &liteIdentity)
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
