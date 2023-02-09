// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateTokenAccount struct{ txnCreate }

var _ chain.SignerValidator = (*CreateTokenAccount)(nil)

func (CreateTokenAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateTokenAccount
}

func (x CreateTokenAccount) Process(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	body, ok := ctx.transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), ctx.transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	err := x.checkCreateAdiAccount(batch, ctx, body.Url)
	if err != nil {
		return nil, err
	}

	if body.TokenUrl == nil {
		return nil, errors.BadRequest.WithFormat("token URL is missing")
	}

	err = verifyCreateTokenAccountProof(&ctx.Executor.Describe, batch, ctx.transaction.Header.Principal, body)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	account := new(protocol.TokenAccount)
	account.Url = body.Url
	account.TokenUrl = body.TokenUrl
	err = x.setAuth(batch, ctx, account)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("set auth: %w", err)
	}

	err = ctx.storeAccount(batch, mustNotExist, account)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create account: %w", err)
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
			return errors.BadRequest.WithFormat("invalid token url %v: %w", body.TokenUrl, err)
		default:
			return errors.UnknownError.WithFormat("load %v: %w", body.TokenUrl, err)
		}

		if account.Type() != protocol.AccountTypeTokenIssuer {
			return errors.BadRequest.WithFormat("invalid token url %v: expected %v, got %v", body.TokenUrl, protocol.AccountTypeTokenIssuer, account.Type())
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

		return errors.BadRequest.With("missing proof of existence for token issuer")
	}

	// Check the proof for missing fields and validity
	proof := body.Proof
	if proof.Transaction == nil {
		return errors.BadRequest.WithFormat("invalid proof: missing transaction")
	}
	if proof.Receipt == nil {
		return errors.BadRequest.WithFormat("invalid proof: missing receipt")
	}
	if !proof.Receipt.Validate() {
		return errors.BadRequest.WithFormat("proof is invalid")
	}

	// Check that the state matches expectations
	if !body.TokenUrl.Equal(proof.Transaction.Url) {
		return errors.BadRequest.With("invalid proof: URL does not match token issuer URL")
	}

	// Check the state hash
	b, err := proof.Transaction.MarshalBinary()
	if err != nil {
		return errors.InternalError.WithFormat("marshal proof state: %v", err)
	}

	hash := sha256.Sum256(b)
	if !bytes.Equal(proof.Receipt.Start, hash[:]) {
		return errors.BadRequest.WithFormat("invalid proof: state hash does not match proof start")
	}

	// Check the anchor - TODO this will not work for the DN
	chain, err := batch.Account(net.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
	if err != nil {
		return errors.InternalError.WithFormat("load anchor pool for directory anchors: %w", err)
	}
	_, err = chain.HeightOf(proof.Receipt.Anchor)
	if err != nil {
		code := errors.UnknownError
		if errors.Is(err, errors.NotFound) {
			code = errors.BadRequest
		}
		return code.WithFormat("invalid proof: lookup DN anchor %X: %w", proof.Receipt.Anchor[:4], err)
	}

	return nil
}
