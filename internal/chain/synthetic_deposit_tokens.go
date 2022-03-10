package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SyntheticDepositTokens struct{}

func (SyntheticDepositTokens) Type() types.TxType {
	return types.TxTypeSyntheticDepositTokens
}

func (SyntheticDepositTokens) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	// *big.Int, tokenChain, *url.URL
	body, ok := tx.Transaction.Body.(*protocol.SyntheticDepositTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositTokens), tx.Transaction.Body)
	}

	var account tokenChain
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteTokenAccount:
			account = origin
		case *protocol.TokenAccount:
			account = origin
		default:
			return nil, fmt.Errorf("invalid origin record: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.GetType())
		}
	} else if keyHash, tok, err := protocol.ParseLiteTokenAddress(tx.Transaction.Origin); err != nil {
		return nil, fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return nil, fmt.Errorf("could not find token account")
	} else if !body.Token.Equal(tok) {
		return nil, fmt.Errorf("token URL does not match lite token account URL")
	} else {
		// Address is lite and the account doesn't exist, so create one
		lite := protocol.NewLiteTokenAccount()
		lite.Url = tx.Transaction.Origin
		lite.TokenUrl = body.Token
		account = lite

		originIdentity := tx.Transaction.Origin.Identity()
		liteIdentity := protocol.NewLiteIdentity()
		err := st.LoadUrlAs(originIdentity, liteIdentity)
		switch {
		case err == nil:
			// OK
		case errors.Is(err, storage.ErrNotFound):
			liteIdentity.Url = originIdentity
			liteIdentity.KeyBook = originIdentity
			st.Update(liteIdentity)
		default:
			return nil, err
		}

		rootIdentity := tx.Transaction.Origin.RootIdentity()
		if rootIdentity.Equal(originIdentity) && !protocol.AcmeUrl().Equal(body.Token) {
			return nil, fmt.Errorf("invalid origin, expecting origin format acc://lite-account/lite-identity/... but got %s", tx.Transaction.Origin.String())
		}
		err = st.AddDirectoryEntry(rootIdentity, tx.Transaction.Origin)
		if err != nil {
			return nil, fmt.Errorf("failed to add directory entries in lite token account %s: %v", tx.Transaction.Origin.RootIdentity(), err)
		}
	}

	if !account.CreditTokens(&body.Amount) {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}
	st.Update(account)

	return nil, nil
}
