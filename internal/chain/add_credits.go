package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type AddCredits struct{}

func (AddCredits) Type() types.TxType { return types.TxTypeAddCredits }

func checkAddCredits(st *StateManager, tx *transactions.GenTransaction) (body *protocol.AddCredits, account tokenChain, amount *types.Amount, recipient *url.URL, err error) {
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

	recv, err := st.LoadUrl(recvUrl)
	if err == nil {
		// If the recipient happens to be on the same BVC, ensure it is a valid
		// recipient. Most credit transfers will be within the same ADI, so this
		// should catch most mistakes early.
		switch recv := recv.(type) {
		case *protocol.AnonTokenAccount, *protocol.SigSpec:
			// OK
		default:
			return nil, nil, nil, nil, fmt.Errorf("invalid recipient: wrong chain type: want %v or %v, got %v", types.ChainTypeAnonTokenAccount, types.ChainTypeSigSpec, recv.Header().Type)
		}
	} else if errors.Is(err, storage.ErrNotFound) {
		if recvUrl.Routing() == tx.Routing {
			// If the recipient and the sponsor have the same routing number,
			// they must be on the same BVC. Thus in that case, failing to
			// locate the recipient chain means it doesn't exist.
			return nil, nil, nil, nil, fmt.Errorf("invalid recipient: not found")
		}
	} else {
		return nil, nil, nil, nil, fmt.Errorf("failed to load recipient: %v", err)
	}

	switch sponsor := st.Sponsor.(type) {
	case *protocol.AnonTokenAccount:
		account = sponsor
	case *state.TokenAccount:
		account = sponsor
	default:
		return nil, nil, nil, nil, fmt.Errorf("not an account: %q", tx.SigInfo.URL)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid token account: %v", err)
	}

	// Only ACME tokens can be converted into credits
	if !protocol.AcmeUrl().Equal(tokenUrl) {
		return nil, nil, nil, nil, fmt.Errorf("%q tokens cannot be converted into credits", tokenUrl.String())
	}

	if !account.CanDebitTokens(&amount.Int) {
		return nil, nil, nil, nil, fmt.Errorf("insufficient balance")
	}

	return body, account, amount, recvUrl, nil
}

func (AddCredits) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, _, err := checkAddCredits(st, tx)
	return err
}

func (AddCredits) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	body, account, amount, recipient, err := checkAddCredits(st, tx)
	if err != nil {
		return err
	}

	if !account.DebitTokens(&amount.Int) {
		return fmt.Errorf("failed to debit %v", tx.SigInfo.URL)
	}
	st.Update(account)

	// Create the synthetic transaction
	sdc := new(protocol.SyntheticDepositCredits)
	sdc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	sdc.Amount = body.Amount
	st.Submit(recipient, sdc)

	return nil
}
