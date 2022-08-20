package chain

import (
	"errors"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type AcmeFaucet struct{}

func (AcmeFaucet) Type() protocol.TransactionType { return protocol.TransactionTypeAcmeFaucet }

func (AcmeFaucet) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (AcmeFaucet{}).Validate(st, tx)
}

func (AcmeFaucet) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Unmarshal the TX payload
	body, ok := tx.Transaction.Body.(*protocol.AcmeFaucet)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.AcmeFaucet), tx.Transaction.Body)
	}

	// Check the recipient
	u := body.Url
	var account *protocol.LiteTokenAccount
	err := st.LoadUrlAs(u, &account)
	switch {
	case err == nil:
		// If the recipient exists, it must be an ACME lite token account
		if !protocol.AcmeUrl().Equal(account.GetTokenUrl()) {
			return nil, fmt.Errorf("invalid recipient: %q is not an ACME account", u)
		}

	case errors.Is(err, storage.ErrNotFound):
		// If the recipient doesn't exist, ensure it is an ACME lite address
		addr, tok, err := protocol.ParseLiteTokenAddress(u)
		switch {
		case err != nil:
			return nil, fmt.Errorf("error parsing lite address %q: %v", u, err)
		case addr == nil:
			return nil, fmt.Errorf("invalid recipient: %q is not a lite address", u)
		case !protocol.AcmeUrl().Equal(tok):
			return nil, fmt.Errorf("invalid recipient: %q is not an ACME account", u)
		}

	default:
		return nil, fmt.Errorf("invalid recipient: %v", err)
	}

	// Load the faucet state
	var faucet *protocol.LiteTokenAccount
	err = st.LoadUrlAs(protocol.FaucetUrl, &faucet)
	if err != nil {
		return nil, fmt.Errorf("failed to load faucet: %v", err)
	}

	// Attach this TX to the faucet (don't bother debiting)
	err = st.Update(faucet)
	if err != nil {
		return nil, fmt.Errorf("failed to update faucet: %v", err)
	}

	// Submit a synthetic deposit token TX
	amount := new(big.Int).SetUint64(protocol.AcmeFaucetAmount * protocol.AcmePrecision)
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *amount
	st.Submit(u, deposit)

	return nil, nil
}
