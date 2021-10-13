package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type AddCredits struct{}

func (AddCredits) Type() types.TxType { return types.TxTypeAddCredits }

func checkAddCredits(st *state.StateEntry, tx *transactions.GenTransaction) (body *protocol.AddCredits, account *state.TokenAccount, amount *types.Amount, recipient *url.URL, err error) {
	if st.ChainHeader == nil {
		return nil, nil, nil, nil, fmt.Errorf("sponsor not found")
	}

	body = new(protocol.AddCredits)
	err = tx.As(body)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	// This fixes the conversion between ACME tokens and fiat currency to
	// 1:1, as in $1 per 1 ACME token.
	//
	// TODO This should be retrieved from an oracle.
	const DollarsPerAcmeToken = 1

	// tokens = credits / (credits per dollar) / (dollars per token)
	amount = types.NewAmount(protocol.AcmePrecision) // Do everything with ACME precision
	amount.Mul(int64(body.Amount))                   // Amount in credits
	amount.Div(protocol.CreditsPerDollar)            // Amount in dollars
	amount.Div(DollarsPerAcmeToken)                  // Amount in tokens

	recvUrl, err := url.Parse(body.Recipient)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid recipient")
	}

	_, recvChain, err := st.DB.LoadChain(recvUrl.ResourceChain())
	if err == nil {
		// If the recipient happens to be on the same BVC, ensure it is a valid
		// recipient. Most credit transfers will be within the same ADI, so this
		// should catch most mistakes early.
		switch recvChain.Type {
		case types.ChainTypeAnonTokenAccount, types.ChainTypeMultiSigSpec:
			// OK
		default:
			return nil, nil, nil, nil, fmt.Errorf("invalid recipient: wrong chain type: want %v or %v, got %v", types.ChainTypeAnonTokenAccount, types.ChainTypeMultiSigSpec, recvChain.Type)
		}
	} else if errors.Is(err, state.ErrNotFound) {
		if recvUrl.Routing() == tx.Routing {
			// If the recipient and the sponsor have the same routing number,
			// they must be on the same BVC. Thus in that case, failing to
			// locate the recipient chain means it doesn't exist.
			return nil, nil, nil, nil, fmt.Errorf("invalid recipient: not found")
		}
	} else {
		return nil, nil, nil, nil, fmt.Errorf("failed to load recipient: %v", err)
	}

	switch st.ChainHeader.Type {
	case types.ChainTypeAnonTokenAccount, types.ChainTypeTokenAccount:
		acct := new(state.TokenAccount)
		err := st.ChainState.As(acct)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("invalid token account: %v", err)
		}

		tokenUrl, err := url.Parse(*acct.TokenUrl.AsString())
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("invalid token account: %v", err)
		}

		// Only ACME tokens can be converted into credits
		if !protocol.AcmeUrl().Equal(tokenUrl) {
			return nil, nil, nil, nil, fmt.Errorf("%q tokens cannot be converted into credits", tokenUrl.String())
		}

		if !acct.CanTransact(&amount.Int) {
			return nil, nil, nil, nil, fmt.Errorf("insufficient balance")
		}

		return body, acct, amount, recvUrl, nil

	default:
		return nil, nil, nil, nil, fmt.Errorf("not an account: %q", tx.SigInfo.URL)
	}
}

func (AddCredits) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, _, _, err := checkAddCredits(st, tx)
	return err
}

func (AddCredits) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	body, account, amount, recipient, err := checkAddCredits(st, tx)
	if err != nil {
		return nil, err
	}

	// Debit the account
	err = account.SubBalance(&amount.Int)
	if err != nil {
		return nil, fmt.Errorf("failed to debit %v", tx.SigInfo.URL)
	}

	st.ChainState.Entry, err = account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	// Create the synthetic transaction
	txid := types.Bytes(tx.TransactionHash()).AsBytes32()
	sdc := new(protocol.SyntheticDepositCredits)
	sdc.Cause = txid
	sdc.Amount = body.Amount

	syn := new(transactions.GenTransaction)
	syn.SigInfo = &transactions.SignatureInfo{}
	syn.SigInfo.URL = recipient.String()
	syn.Transaction, err = sdc.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	// Update the account state
	st.DB.AddStateEntry(st.ChainId, &txid, st.ChainState)

	res := new(DeliverTxResult)
	res.AddSyntheticTransaction(syn)
	return res, nil
}
