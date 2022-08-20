package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
		return nil, errors.Internal.Format("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
	}

	err := checkCreateAdiAccount(st, body.Url)
	if err != nil {
		return nil, err
	}

	if body.TokenUrl == nil {
		return nil, errors.BadRequest.Format("token URL is missing")
	}

	err = verifyCreateTokenAccountProof(st.Describe, st.batch, tx.Transaction.Header.Principal, body)
	if err != nil {
		return nil, errors.Unknown.Wrap(err)
	}

	account := new(protocol.TokenAccount)
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	err = st.SetAuth(account, body.Authorities)
	if err != nil {
		return nil, errors.Unknown.Format("set auth: %w", err)
	}

	err = st.Create(account)
	if err != nil {
		return nil, errors.Unknown.Format("create account: %w", err)
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
		case errors.Is(err, errors.NotFound):
			return errors.BadRequest.Format("invalid token url %v: %w", body.TokenUrl, err)
		default:
			return errors.Unknown.Format("load %v: %w", body.TokenUrl, err)
		}

		if account.Type() != protocol.AccountTypeTokenIssuer {
			return errors.BadRequest.Format("invalid token url %v: expected %v, got %v", body.TokenUrl, protocol.AccountTypeTokenIssuer, account.Type())
		}
	}

	// Check that the proof is present if required
	if body.Proof == nil {
		// Proof is not required for ACME
		if body.TokenUrl.Equal(protocol.AcmeUrl()) {
			return nil
		}

		// Proof is not required for local issuers
		if local {
			return nil
		}

		return errors.BadRequest.New("missing proof of existence for token issuer")
	}

	// Check the proof for missing fields and validity
	proof := body.Proof
	if proof.Transaction == nil {
		return errors.BadRequest.Format("invalid proof: missing transaction")
	}
	if proof.Receipt == nil {
		return errors.BadRequest.Format("invalid proof: missing receipt")
	}
	if !proof.Receipt.Validate() {
		return errors.BadRequest.Format("proof is invalid")
	}

	// Check that the state matches expectations
	if !body.TokenUrl.Equal(proof.Transaction.Url) {
		return errors.BadRequest.New("invalid proof: URL does not match token issuer URL")
	}

	// Check the state hash
	b, err := proof.Transaction.MarshalBinary()
	if err != nil {
		return errors.Internal.Format("marshal proof state: %v", err)
	}

	hash := sha256.Sum256(b)
	if !bytes.Equal(proof.Receipt.Start, hash[:]) {
		return errors.BadRequest.Format("invalid proof: state hash does not match proof start")
	}

	// Check the anchor - TODO this will not work for the DN
	chain, err := batch.Account(net.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
	if err != nil {
		return errors.Internal.Format("load anchor pool for directory anchors: %w", err)
	}
	_, err = chain.HeightOf(proof.Receipt.Anchor)
	if err != nil {
		code := errors.Unknown
		if errors.Is(err, errors.NotFound) {
			code = errors.BadRequest
		}
		return code.Format("invalid proof: lookup DN anchor %X: %w", proof.Receipt.Anchor[:4], err)
	}

	return nil
}
