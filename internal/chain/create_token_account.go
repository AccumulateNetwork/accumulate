package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateTokenAccount struct{}

func (CreateTokenAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateTokenAccount
}

func (CreateTokenAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateTokenAccount{}).Validate(st, tx)
}

func (CreateTokenAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
	}

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q cannot be the origininator of %q", st.OriginUrl, body.Url)
	}

	account := new(protocol.TokenAccount)
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	account.Scratch = body.Scratch
	acc := st.batch.Account(account.Url)
	mReceipt, err := acc.StateReceipt()
	if err != nil {
		return nil, err
	}
	if body.AccState != nil && body.Proof != nil {
		if !bytes.Equal(mReceipt.Element, body.AccState) || !bytes.Equal(mReceipt.MDRoot, body.Proof) {
			return nil, fmt.Errorf("invalid accounturl state cannot be verified")
		}
	}
	/*accst, err := acc.GetState()
	if err != nil {
		return nil, err
	}

	accstb, err := accst.MarshalBinary()
	if err != nil {
		return nil, err
	}
	acchash := sha256.Sum256(accstb)
	if !bytes.Equal(acchash[:], body.AccState) {
		return nil, fmt.Errorf("invalid accounturl state cannot be verified")
	}
	st.batch.BptReceipt()*/
	err = st.SetAuth(account, body.KeyBookUrl, body.Manager)
	if err != nil {
		return nil, err
	}

	err = st.Create(account)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", account.Url, err)
	}
	return nil, nil
}
