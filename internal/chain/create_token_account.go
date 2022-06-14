package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateTokenAccount struct{}

var _ SignerValidator = (*CreateTokenAccount)(nil)

func (CreateTokenAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateTokenAccount
}

func (CreateTokenAccount) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, signer, checkAuthz)
}

func (CreateTokenAccount) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
}

func (CreateTokenAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateTokenAccount{}).Validate(st, tx)
}

func (CreateTokenAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
	}

	err := checkCreateAdiAccount(st, body.Url)
	if err != nil {
		return nil, err
	}

	if body.TokenUrl == nil {
		return nil, errors.Format(errors.StatusBadRequest, "token URL is missing")
	}

	err = verifyCreateTokenAccountProof(st.Describe, st.batch, tx.Transaction.Header.Principal, body)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	account := new(protocol.TokenAccount)
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	account.Scratch = body.Scratch
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

func verifyCreateTokenAccountProof(net *config.Describe, batch *database.Batch, principal *url.URL, body *protocol.CreateTokenAccount) error {
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
	if proof.Proof == nil {
		return errors.Format(errors.StatusBadRequest, "invalid proof: missing Proof")
	}
	if !proof.Proof.Validate() {
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
	if !bytes.Equal(proof.Proof.Start, stateHash[:]) {
		return errors.Format(errors.StatusBadRequest, "invalid proof: state hash does not match proof start")
	}

	// Check the anchor - TODO this will not work for the DN
	chain, err := batch.Account(net.AnchorPool()).ReadChain(protocol.RootAnchorChain(protocol.Directory))
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load anchor pool for directory anchors: %w", err)
	}
	_, err = chain.HeightOf(proof.Proof.Anchor)
	if err != nil {
		code := errors.StatusUnknown
		if errors.Is(err, errors.StatusNotFound) {
			code = errors.StatusBadRequest
		}
		return errors.Format(code, "invalid proof: lookup DN anchor %X: %w", proof.Proof.Anchor[:4], err)
	}

	return nil
}
