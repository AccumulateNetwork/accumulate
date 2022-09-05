package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateDataAccount struct{}

var _ SignerValidator = (*CreateDataAccount)(nil)

func (CreateDataAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateDataAccount
}

func (CreateDataAccount) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateDataAccount)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateDataAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, signer, md)
}

func (CreateDataAccount) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateDataAccount)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateDataAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
}

func (CreateDataAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateDataAccount{}).Validate(st, tx)
}

func (CreateDataAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateDataAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateDataAccount), tx.Transaction.Body)
	}

	err := checkCreateAdiAccount(st, body.Url)
	if err != nil {
		return nil, err
	}

	//create the data account
	account := new(protocol.DataAccount)
	account.Url = body.Url

	err = st.SetAuth(account, body.Authorities)
	if err != nil {
		return nil, err
	}

	err = st.Create(account)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", account.Url, err)
	}
	return nil, nil
}
