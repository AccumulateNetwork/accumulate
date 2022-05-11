package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateTokenAccount struct{}

var _ SignerValidator = (*CreateTokenAccount)(nil)

func (CreateTokenAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateTokenAccount
}

func (CreateTokenAccount) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, location *url.URL, signer protocol.Signer) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, location, signer)
}

func (CreateTokenAccount) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
}

func (CreateTokenAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateTokenAccount{}).Validate(st, tx)
}

func (CreateTokenAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
	}

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q cannot be the origininator of %q", st.OriginUrl, body.Url)
	}

	account := new(protocol.TokenAccount)
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	account.Scratch = body.Scratch

	err := st.SetAuth(account, body.Authorities)
	if err != nil {
		return nil, err
	}

	err = st.Create(account)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", account.Url, err)
	}
	return nil, nil
}
