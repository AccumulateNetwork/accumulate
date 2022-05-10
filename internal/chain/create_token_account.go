package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
	proof := body.TokenIssuerProof
	fmt.Println(proof)
	if proof != nil {
		if proof.State.Type() != protocol.AccountTypeTokenIssuer {
			return nil, fmt.Errorf("Account state cannot be verified")
		}
		var act *protocol.TokenIssuer
		var err error
		act = proof.State.(*protocol.TokenIssuer)
		accBytes, _ := act.MarshalBinary()
		accStateHash := sha256.Sum256(accBytes)
		var anchorChain *database.Chain
		anchorpath := protocol.DnUrl().JoinPath(protocol.AnchorPool)
		anchorChain, err = st.batch.Account(st.NodeUrl()).ReadChain(anchorpath.String())
		if err != nil {
			return nil, fmt.Errorf("Error reading achor chain: %x", err)
		}
		_, err = anchorChain.HeightOf(proof.Receipt.Result)
		if err != nil {
			return nil, fmt.Errorf("Account state cannot be verified: %x", err)
		}

		if bytes.Compare(accStateHash[:], proof.Receipt.Start) != 0 || err != nil {
			return nil, fmt.Errorf("Account state cannot be verified")
		}
	}
	err := st.SetAuth(account, body.Authorities)
	if err != nil {
		return nil, err
	}

	err = st.Create(account)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", account.Url, err)
	}
	return nil, nil
}
