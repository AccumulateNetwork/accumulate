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

type AddCredits struct{}

func (AddCredits) Type() types.TxType { return types.TxTypeAddCredits }

func (AddCredits) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.AddCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.AddCredits), tx.Transaction.Body)
	}

	ledgerState := protocol.NewInternalLedger()
	err := st.LoadUrlAs(st.nodeUrl.JoinPath(protocol.Ledger), ledgerState)
	if err != nil {
		return nil, err
	}

	if ledgerState.ActiveOracle == 0 {
		return nil, fmt.Errorf("cannot purchase credits: acme oracle price has not been set")
	}

	// If specifying amount of acme to spend
	credits := big.NewInt(protocol.CreditsPerFiatUnit)                    // want to obtain credits
	credits.Mul(credits, big.NewInt(int64(ledgerState.ActiveOracle)))     // fiat units / acme
	credits.Mul(credits, &body.Amount)                                    // acme the user wants to spend
	credits.Div(credits, big.NewInt(int64(protocol.AcmeOraclePrecision))) // adjust the precision of oracle to real units
	credits.Div(credits, big.NewInt(int64(protocol.AcmePrecision)))       // adjust the precision of acme to spend to real units

	recv, err := st.LoadUrl(body.Recipient)
	if err == nil {
		// If the recipient happens to be on the same BVC, ensure it is a valid
		// recipient. Most credit transfers will be within the same ADI, so this
		// should catch most mistakes early.
		switch recv := recv.(type) {
		case *protocol.LiteTokenAccount, *protocol.KeyPage:
			// OK
		default:
			return nil, fmt.Errorf("invalid recipient: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeKeyPage, recv.GetType())
		}
	} else if errors.Is(err, storage.ErrNotFound) {
		if body.Recipient.Routing() == tx.Transaction.Origin.Routing() {
			// If the recipient and the origin have the same routing number,
			// they must be on the same BVC. Thus in that case, failing to
			// locate the recipient chain means it doesn't exist.
			return nil, fmt.Errorf("invalid recipient: not found")
		}
	} else {
		return nil, fmt.Errorf("failed to load recipient: %v", err)
	}

	var account tokenChain
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin
	case *protocol.TokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("not an account: %q", tx.Transaction.Origin)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return nil, fmt.Errorf("invalid token account: %v", err)
	}

	// Only ACME tokens can be converted into credits
	if !protocol.AcmeUrl().Equal(tokenUrl) {
		return nil, fmt.Errorf("%q tokens cannot be converted into credits", tokenUrl.String())
	}

	if !account.CanDebitTokens(&body.Amount) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), &body.Amount)
	}

	if !account.DebitTokens(&body.Amount) {
		return nil, fmt.Errorf("failed to debit %v", tx.Transaction.Origin)
	}
	if credits == big.NewInt(0) {
		return nil, fmt.Errorf("%v credits", credits)
	}
	st.Update(account)

	// Create the synthetic transaction
	sdc := new(protocol.SyntheticDepositCredits)
	copy(sdc.Cause[:], tx.GetTxHash())
	sdc.Amount = *credits
	st.Submit(body.Recipient, sdc)

	//Create synthetic burn token
	burnAcme := new(protocol.SyntheticBurnTokens)
	copy(sdc.Cause[:], tx.GetTxHash())
	burnAcme.Amount = body.Amount
	st.Submit(tokenUrl, burnAcme)

	return nil, nil
}
