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

func (AddCredits) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.AddCredits)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	// tokens = credits / (credits per dollar) / (dollars per token)
	amount := types.NewAmount(protocol.AcmePrecision) // Do everything with ACME precision
	amount.Mul(int64(body.Amount))                    // Amount in credits
	amount.Div(protocol.CreditsPerFiatUnit)           // Amount in dollars
	amount.Div(protocol.FiatUnitsPerAcmeToken)        // Amount in tokens

	recvUrl, err := url.Parse(body.Recipient)
	if err != nil {
		return fmt.Errorf("invalid recipient")
	}

	recv, err := st.LoadUrl(recvUrl)
	if err == nil {
		// If the recipient happens to be on the same BVC, ensure it is a valid
		// recipient. Most credit transfers will be within the same ADI, so this
		// should catch most mistakes early.
		switch recv := recv.(type) {
		case *protocol.LiteTokenAccount, *protocol.KeyPage:
			// OK
		default:
			return fmt.Errorf("invalid recipient: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeKeyPage, recv.Header().Type)
		}
	} else if errors.Is(err, storage.ErrNotFound) {
		if recvUrl.Routing() == tx.Routing {
			// If the recipient and the sponsor have the same routing number,
			// they must be on the same BVC. Thus in that case, failing to
			// locate the recipient chain means it doesn't exist.
			return fmt.Errorf("invalid recipient: not found")
		}
	} else {
		return fmt.Errorf("failed to load recipient: %v", err)
	}

	var account tokenChain
	switch sponsor := st.Sponsor.(type) {
	case *protocol.LiteTokenAccount:
		account = sponsor
	case *state.TokenAccount:
		account = sponsor
	default:
		return fmt.Errorf("not an account: %q", tx.SigInfo.URL)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return fmt.Errorf("invalid token account: %v", err)
	}

	// Only ACME tokens can be converted into credits
	if !protocol.AcmeUrl().Equal(tokenUrl) {
		return fmt.Errorf("%q tokens cannot be converted into credits", tokenUrl.String())
	}

	if !account.CanDebitTokens(&amount.Int) {
		return fmt.Errorf("insufficient balance")
	}

	if !account.DebitTokens(&amount.Int) {
		return fmt.Errorf("failed to debit %v", tx.SigInfo.URL)
	}
	st.Update(account)

	// Create the synthetic transaction
	sdc := new(protocol.SyntheticDepositCredits)
	sdc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	sdc.Amount = body.Amount
	st.Submit(recvUrl, sdc)

	return nil
}
