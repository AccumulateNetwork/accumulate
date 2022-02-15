package chain

import (
	"errors"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type AcmeFaucet struct{}

func (AcmeFaucet) Type() types.TxType { return types.TxTypeAcmeFaucet }

func (AcmeFaucet) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	// Unmarshal the TX payload
	body, ok := tx.Transaction.Body.(*protocol.AcmeFaucet)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.AcmeFaucet), tx.Transaction.Body)
	}

	// Check the recipient
	u := body.Url
	account := new(protocol.LiteTokenAccount)
	err := st.LoadUrlAs(u, account)
	switch {
	case err == nil:
		// If the recipient exists, it must be an ACME lite token account
		u, err := account.ParseTokenUrl()
		if err != nil {
			return nil, fmt.Errorf("invalid record: bad token URL: %v", err)
		}

		if !protocol.AcmeUrl().Equal(u) {
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
	faucet := new(protocol.LiteTokenAccount)
	err = st.LoadUrlAs(protocol.FaucetUrl, faucet)
	if err != nil {
		return nil, fmt.Errorf("failed to load faucet: %v", err)
	}

	// Attach this TX to the faucet (don't bother debiting)
	st.Update(faucet)

	// Submit a synthetic deposit token TX
	amount := new(big.Int).SetUint64(100 * protocol.AcmePrecision)
	deposit := new(protocol.SyntheticDepositTokens)
	copy(deposit.Cause[:], tx.GetTxHash())
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *amount
	st.Submit(u, deposit)

	// deposit := synthetic.NewTokenTransactionDeposit(txid[:], types.String(protocol.FaucetUrl.String()), types.String(u.String()))
	// err = deposit.SetDeposit(protocol.ACME, amount)

	return nil, nil
}
