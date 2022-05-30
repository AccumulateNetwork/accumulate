package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticDepositCredits struct{}

var _ PrincipalValidator = (*SyntheticDepositCredits)(nil)

func (SyntheticDepositCredits) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticDepositCredits
}

func (SyntheticDepositCredits) AllowMissingPrincipal(transaction *protocol.Transaction) (allow, fallback bool) {
	// The principal can be missing if it is a lite identity
	key, _ := protocol.ParseLiteIdentity(transaction.Header.Principal)
	return key != nil, false
}

func (SyntheticDepositCredits) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticDepositCredits{}).Validate(st, tx)
}

func (SyntheticDepositCredits) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticDepositCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticDepositCredits), tx.Transaction.Body)
	}

	var account protocol.Signer
	var create bool
	switch origin := st.Origin.(type) {
	case nil:
		// Create a new lite identity
		create = true
		key, _ := protocol.ParseLiteIdentity(tx.Transaction.Header.Principal)
		if key == nil {
			return nil, errors.NotFound("%v not found", tx.Transaction.Header.Principal)
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

	var err error
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
