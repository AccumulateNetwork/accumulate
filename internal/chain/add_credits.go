package chain

import (
	"errors"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

var bigZeroAmount = big.NewInt(0)

type AddCredits struct{}

func (AddCredits) Type() protocol.TransactionType { return protocol.TransactionTypeAddCredits }

func (AddCredits) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.AddCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.AddCredits), tx.Transaction.Body)
	}

	// need to make sure acme amount is greater than 0
	if body.Amount.Cmp(bigZeroAmount) <= 0 {
		return nil, fmt.Errorf("invalid ACME amount specified for credit purchase, must be greater than zero")
	}

	// need to make sure oracle is set
	if body.Oracle == 0 {
		return nil, fmt.Errorf("cannot purchase credits, transaction oracle must be greater than zero")
	}

	var ledgerState *protocol.InternalLedger
	err := st.LoadUrlAs(st.nodeUrl.JoinPath(protocol.Ledger), &ledgerState)
	if err != nil {
		return nil, err
	}

	// need to make sure internal oracle is set.
	if ledgerState.ActiveOracle == 0 {
		return nil, fmt.Errorf("cannot purchase credits: acme oracle price has not been set")
	}

	// make sure oracle matches
	if body.Oracle != ledgerState.ActiveOracle {
		return nil, fmt.Errorf("oracle doesn't match")
	}

	// If specifying amount of acme to spend
	credits := big.NewInt(int64(protocol.CreditUnitsPerFiatUnit * ledgerState.ActiveOracle))     // credit units / dollar * oracle precision * dollar / acme
	credits.Mul(credits, &body.Amount)                                                           // acme the user wants to spend - acme * acme precision
	credits.Div(credits, big.NewInt(int64(protocol.AcmeOraclePrecision*protocol.AcmePrecision))) // adjust the precision of oracle to real units - oracle precision and acme to spend with acme precision

	//make sure there are credits to purchase
	if credits.Int64() == 0 {
		return nil, fmt.Errorf("no credits can be purchased with specified ACME amount %v", body.Amount)
	}

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
		if body.Recipient.Routing() == tx.Transaction.Header.Principal.Routing() {
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
		return nil, fmt.Errorf("not an account: %q", tx.Transaction.Header.Principal)
	}

	// Only ACME tokens can be converted into credits
	if !protocol.AcmeUrl().Equal(account.GetTokenUrl()) {
		return nil, fmt.Errorf("%q tokens cannot be converted into credits", account.GetTokenUrl())
	}

	if !account.CanDebitTokens(&body.Amount) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), &body.Amount)
	}

	if !account.DebitTokens(&body.Amount) {
		return nil, fmt.Errorf("failed to debit %v", tx.Transaction.Header.Principal)
	}

	st.Update(account)

	// Create the synthetic transaction
	sdc := new(protocol.SyntheticDepositCredits)
	copy(sdc.Cause[:], tx.GetTxHash())
	sdc.Amount = credits.Uint64()
	st.Submit(body.Recipient, sdc)

	//Create synthetic burn token
	/*	burnAcme := new(protocol.SyntheticBurnTokens)
		copy(sdc.Cause[:], tx.GetTxHash())
		burnAcme.Amount = body.Amount
		st.Submit(account.GetTokenUrl(), burnAcme)*/

	ledgerState.AcmeBurnt = *ledgerState.AcmeBurnt.Add(&ledgerState.AcmeBurnt, &body.Amount)
	st.Update(ledgerState)

	res := new(protocol.AddCreditsResult)
	res.Oracle = ledgerState.ActiveOracle
	res.Credits = credits.Uint64()
	res.Amount = body.Amount
	return res, nil
}
