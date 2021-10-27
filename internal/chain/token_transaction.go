package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type TokenTx struct{}

func (TokenTx) Type() types.TxType { return types.TxTypeTokenTx }

type tokenSend struct {
	url    *url.URL
	amount *big.Int
}

func checkTokenTx(st *StateManager, tx *transactions.GenTransaction) ([]*tokenSend, tokenChain, *url.URL, *big.Int, error) {
	body := new(api.TokenTx)
	err := tx.As(body)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	if body.From.String != types.String(tx.SigInfo.URL) {
		return nil, nil, nil, nil, fmt.Errorf("withdraw address and transaction sponsor do not match")
	}

	sends := make([]*tokenSend, len(body.To))
	for i, to := range body.To {
		u, err := url.Parse(*to.URL.AsString())
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("invalid destination URL: %v", err)
		}

		amount := new(big.Int)
		amount.SetUint64(to.Amount)
		sends[i] = &tokenSend{u, amount}
	}

	var account tokenChain
	switch sponsor := st.Sponsor.(type) {
	case *state.TokenAccount:
		account = sponsor
	case *protocol.AnonTokenAccount:
		account = sponsor
	default:
		return nil, nil, nil, nil, fmt.Errorf("%v cannot sponsor token transactions", st.Sponsor.Header().Type)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("invalid token URL: %v", err)
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	amt := types.Amount{}
	txAmt := big.NewInt(0)
	for _, val := range body.To {
		amt.Add(amt.AsBigInt(), txAmt.SetUint64(val.Amount))
	}

	if !account.CanDebitTokens(&amt.Int) {
		return nil, nil, nil, nil, fmt.Errorf("insufficient balance")
	}

	return sends, account, tokenUrl, &amt.Int, nil
}

func (c TokenTx) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, _, err := checkTokenTx(st, tx)
	return err
}

func (c TokenTx) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	body, account, tokenUrl, debit, err := checkTokenTx(st, tx)
	if err != nil {
		return err
	}

	token := types.String(tokenUrl.String())
	txid := types.Bytes(tx.TransactionHash())
	for _, send := range body {
		from := types.String(st.SponsorUrl.String())
		to := types.String(send.url.String())
		deposit := synthetic.NewTokenTransactionDeposit(txid[:], from, to)
		err = deposit.SetDeposit(token, send.amount)
		if err != nil {
			return fmt.Errorf("invalid deposit: %v", err)
		}

		st.Submit(send.url, deposit)
	}

	if !account.DebitTokens(debit) {
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
