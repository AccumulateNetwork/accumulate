// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var bigZeroAmount = big.NewInt(0)

type AddCredits struct{}

func (AddCredits) Type() protocol.TransactionType { return protocol.TransactionTypeAddCredits }

func (AddCredits) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (AddCredits{}).Validate(st, tx)
}

func (AddCredits) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.AddCredits)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.AddCredits), tx.Transaction.Body)
	}

	if body.Recipient == nil {
		return nil, fmt.Errorf("invalid payload: missing recipient")
	}

	// need to make sure acme amount is greater than 0
	if body.Amount.Cmp(bigZeroAmount) <= 0 {
		return nil, fmt.Errorf("invalid ACME amount specified for credit purchase, must be greater than zero")
	}

	// need to make sure oracle is set
	if body.Oracle == 0 {
		return nil, fmt.Errorf("cannot purchase credits, transaction oracle must be greater than zero")
	}

	// need to make sure internal oracle is set.
	if st.Globals.Oracle.Price == 0 {
		return nil, fmt.Errorf("cannot purchase credits: acme oracle price has not been set")
	}

	// make sure oracle matches
	if body.Oracle != st.Globals.Oracle.Price {
		return nil, fmt.Errorf("oracle doesn't match")
	}

	// minimum spend (ACME) = minimum spend (credits) * dollars/credit ÷ dollars/ACME
	minSpend := new(big.Int)
	minSpend.SetUint64(protocol.MinimumCreditPurchase.AsUInt64() * protocol.AcmeOraclePrecision * protocol.AcmePrecision)
	minSpend.Div(minSpend, big.NewInt(int64(protocol.CreditUnitsPerFiatUnit*st.Globals.Oracle.Price)))
	if body.Amount.Cmp(minSpend) < 0 {
		return nil, fmt.Errorf("amount is less than minimum")
	}

	// Calculate credits from spend (ACME)
	credits := big.NewInt(int64(protocol.CreditUnitsPerFiatUnit * st.Globals.Oracle.Price))      // credit units / dollar * oracle precision * dollar / acme
	credits.Mul(credits, &body.Amount)                                                           // acme the user wants to spend - acme * acme precision
	credits.Div(credits, big.NewInt(int64(protocol.AcmeOraclePrecision*protocol.AcmePrecision))) // adjust the precision of oracle to real units - oracle precision and acme to spend with acme precision

	//make sure there are credits to purchase
	if credits.Int64() == 0 {
		return nil, fmt.Errorf("no credits can be purchased with specified ACME amount %v", body.Amount)
	}

	var account protocol.AccountWithTokens
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin
	case *protocol.TokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("not a token account: %q", tx.Transaction.Header.Principal)
	}

	// Only ACME tokens can be converted into credits
	if !protocol.AcmeUrl().Equal(account.GetTokenUrl()) {
		return nil, fmt.Errorf("%q tokens cannot be converted into credits", account.GetTokenUrl())
	}

	if !account.DebitTokens(&body.Amount) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), &body.Amount)
		// return nil, fmt.Errorf("insufficient balance: have %v, want %v",
		// 	protocol.FormatBigAmount(account.TokenBalance(), protocol.AcmePrecisionPower),
		// 	protocol.FormatBigAmount(&body.Amount, protocol.AcmePrecisionPower))
	}

	// Convert a lite token account recipient into a lite identity. Do not check
	// if the account exists, that will be done by the deposit.
	recipient := body.Recipient
	if key, _, _ := protocol.ParseLiteTokenAddress(recipient); key != nil {
		recipient = recipient.RootIdentity()
	}

	// Create the synthetic transaction
	sdc := new(protocol.SyntheticDepositCredits)
	sdc.Amount = credits.Uint64()
	if body.Amount.Cmp(minSpend) > 0 {
		sdc.AcmeRefundAmount = new(big.Int).Sub(&body.Amount, minSpend)
	}
	st.Submit(recipient, sdc)

	var ledgerState *protocol.SystemLedger
	err := st.LoadUrlAs(st.NodeUrl(protocol.Ledger), &ledgerState)
	if err != nil {
		return nil, err
	}

	// Add the burnt acme to the internal ledger and send it with the anchor
	// transaction
	ledgerState.AcmeBurnt.Add(&ledgerState.AcmeBurnt, minSpend)

	res := new(protocol.AddCreditsResult)
	res.Oracle = st.Globals.Oracle.Price
	res.Credits = credits.Uint64()
	res.Amount = body.Amount

	err = st.Update(account, ledgerState)
	if err != nil {
		return nil, fmt.Errorf("failed to update accounts: %v", err)
	}

	return res, nil
}
