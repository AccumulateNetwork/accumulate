package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateKeyBook struct{}

var _ SignerValidator = (*CreateKeyBook)(nil)

func (CreateKeyBook) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyBook }

func (CreateKeyBook) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, signer, checkAuthz)
}

func (CreateKeyBook) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
}

func (CreateKeyBook) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateKeyBook{}).Validate(st, tx)
}

func (CreateKeyBook) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeIdentity, st.Origin.Type())
	}

	body, ok := tx.Transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), tx.Transaction.Body)
	}

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q does not belong to %q", body.Url, st.OriginUrl)
	}

	book := new(protocol.KeyBook)
	book.Url = body.Url
	book.AddAuthority(body.Url)
	book.PageCount = 1

	err := st.SetAuth(book, body.Authorities)
	if err != nil {
		return nil, err
	}

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(body.Url, 0)

	key := new(protocol.KeySpec)
	key.PublicKeyHash = body.PublicKeyHash
	page.Keys = []*protocol.KeySpec{key}

	err = st.Create(book, page)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", book.Url, err)
	}
	return nil, nil
}
