package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
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
		return nil, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
	}

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, errors.Format(errors.StatusBadRequest, "%q cannot be the origininator of %q", st.OriginUrl, body.Url)
	}

	err := verifyCreateTokenAccountProof(st.Network, st.batch, tx.Transaction.Header.Principal, body)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	account := new(protocol.TokenAccount)
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	account.Scratch = body.Scratch
	proof := body.TokenIssuerProof
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

	err = st.SetAuth(account, body.Authorities)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "set auth: %w", err)
	}

	err = st.Create(account)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "create account: %w", err)
	}
	return nil, nil
}

func verifyCreateTokenAccountProof(net *config.Network, batch *database.Batch, principal *url.URL, body *protocol.CreateTokenAccount) error {
	// If the issuer is local, check if it exists
	local := principal.LocalTo(body.TokenUrl)
	if local {
		account, err := batch.Account(body.TokenUrl).GetState()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.StatusNotFound):
			return errors.Format(errors.StatusBadRequest, "invalid token url %v: %w", body.TokenUrl, err)
		default:
			return errors.Format(errors.StatusUnknown, "load %v: %w", body.TokenUrl, err)
		}

		if account.Type() != protocol.AccountTypeTokenIssuer {
			return errors.Format(errors.StatusBadRequest, "invalid token url %v: expected %v, got %v", body.TokenUrl, protocol.AccountTypeTokenIssuer, account.Type())
		}
	}

	// Check that the proof is present if required
	proof := body.TokenIssuerProof
	if proof == nil {
		// Proof is not required for ACME
		if body.TokenUrl.Equal(protocol.AcmeUrl()) {
			return nil
		}

		// Proof is not required for local issuers
		if local {
			return nil
		}

		return errors.New(errors.StatusBadRequest, "missing proof of existence for token issuer")
	}

	// Check the proof for missing fields and validity
	if proof.State == nil {
		return errors.Format(errors.StatusBadRequest, "invalid proof: missing state")
	}
	if proof.Receipt == nil {
		return errors.Format(errors.StatusBadRequest, "invalid proof: missing receipt")
	}
	if !proof.Receipt.Convert().Validate() {
		return errors.Format(errors.StatusBadRequest, "proof is invalid")
	}

	// Check that the state matches expectations
	_, ok := proof.State.(*protocol.TokenIssuer)
	if !ok {
		return errors.Format(errors.StatusBadRequest, "invalid proof state: expected %v, got %v", protocol.AccountTypeTokenIssuer, proof.State.Type())
	}
	if !body.TokenUrl.Equal(proof.State.GetUrl()) {
		return errors.New(errors.StatusBadRequest, "invalid proof state: URL does not match token issuer URL")
	}

	// Check the state hash
	stateBytes, err := proof.State.MarshalBinary()
	if err != nil {
		return errors.Format(errors.StatusInternalError, "marshal proof state: %v", err)
	}

	stateHash := sha256.Sum256(stateBytes)
	if !bytes.Equal(proof.Receipt.Start, stateHash[:]) {
		return errors.Format(errors.StatusBadRequest, "invalid proof: state hash does not match proof start")
	}

	// Check the anchor - TODO this will not work for the DN
	chain, err := batch.Account(net.AnchorPool()).ReadChain(protocol.AnchorChain(protocol.Directory))
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load anchor pool for directory anchors: %w", err)
	}
	_, err = chain.HeightOf(proof.Receipt.Result)
	if err != nil {
		code := errors.StatusUnknown
		if errors.Is(err, errors.StatusNotFound) {
			code = errors.StatusBadRequest
		}
		return errors.Format(code, "invalid proof: lookup DN anchor %X: %w", proof.Receipt.Result[:4], err)
	}

	return nil
}
