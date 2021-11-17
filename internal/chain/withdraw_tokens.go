package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api"

	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
)

type WithdrawTokens struct{}

func (WithdrawTokens) Type() types.TxType { return types.TxTypeWithdrawTokens }

func (WithdrawTokens) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(api.TokenTx)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if body.From.String != types.String(tx.SigInfo.URL) {
		return fmt.Errorf("withdraw address and transaction sponsor do not match")
	}

	recipients := make([]*url.URL, len(body.To))
	for i, to := range body.To {
		recipients[i], err = url.Parse(*to.URL.AsString())
		if err != nil {
			return fmt.Errorf("invalid destination URL: %v", err)
		}
	}

	var account tokenChain
	switch sponsor := st.Sponsor.(type) {
	case *state.TokenAccount:
		account = sponsor
	case *protocol.AnonTokenAccount:
		account = sponsor
	default:
		return fmt.Errorf("invalid sponsor: want %v or %v, got %v", types.ChainTypeTokenAccount, types.ChainTypeLiteTokenAccount, st.Sponsor.Header().Type)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return fmt.Errorf("invalid token URL: %v", err)
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	total := types.Amount{}
	for _, to := range body.To {
		total.Add(total.AsBigInt(), new(big.Int).SetUint64(to.Amount))
	}

	if !account.CanDebitTokens(&total.Int) {
		return fmt.Errorf("insufficient balance")
	}

	token := types.String(tokenUrl.String())
	txid := types.Bytes(tx.TransactionHash())
	for i, u := range recipients {
		from := types.String(st.SponsorUrl.String())
		to := types.String(u.String())
		deposit := synthetic.NewTokenTransactionDeposit(txid[:], from, to)
		err = deposit.SetDeposit(token, new(big.Int).SetUint64(body.To[i].Amount))
		if err != nil {
			return fmt.Errorf("invalid deposit: %v", err)
		}

		st.Submit(u, deposit)
	}

	if !account.DebitTokens(&total.Int) {
		return fmt.Errorf("%q balance is insufficient", st.SponsorUrl)
	}
	st.Update(account)

	txHash := txid.AsBytes32()
	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := st.SponsorUrl.JoinPath(fmt.Sprint(account.NextTx()))
	txr := state.NewTxReference(refUrl.String(), txHash[:])
	st.Update(txr)

	return nil
}
