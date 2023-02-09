// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[CreateIdentity](&transactionExecutors, protocol.TransactionTypeCreateIdentity)
}

type CreateIdentity struct{ txnCreate }

var _ chain.SignerValidator = (*CreateIdentity)(nil)
var _ chain.PrincipalValidator = (*CreateIdentity)(nil)

func (x CreateIdentity) SignerIsAuthorized(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md chain.SignatureValidationMetadata) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return false, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateIdentity), transaction.Body)
	}

	// Anyone is allowed to sign for a root identity
	if body.Url.IsRootIdentity() {
		return false, nil
	}

	// Check additional authorities
	return x.txnCreate.SignerIsAuthorized(delegate, batch, transaction, signer, md)
}

func (x CreateIdentity) TransactionIsReady(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return false, false, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateIdentity), transaction.Body)
	}

	// Check additional authorities
	ready, fallback, err = x.txnCreate.TransactionIsReady(delegate, batch, transaction, status)
	if !fallback || err != nil {
		return ready, fallback, err
	}

	// Anyone is allowed to sign for a root identity
	if body.Url.IsRootIdentity() {
		return true, false, nil
	}

	// Fallback to general authorization
	return false, true, nil
}

func (CreateIdentity) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// The principal can be missing if it is a root identity
	return transaction.Header.Principal.IsRootIdentity()
}

func (x CreateIdentity) Process(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	body, ok := ctx.transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid transaction: want %v, got %v", protocol.TransactionTypeCreateIdentity, ctx.transaction)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	// TODO Require the principal to be the ADI when creating a root identity?
	if !body.Url.IsRootIdentity() {
		err := x.checkCreateAdiAccount(batch, ctx, body.Url)
		if err != nil {
			return nil, err
		}
	}

	if body.KeyBookUrl == nil && len(body.Authorities) == 0 && body.Url.IsRootIdentity() {
		return nil, errors.BadRequest.WithFormat("a root identity cannot be created with an empty authority set")
	}

	err := protocol.IsValidAdiUrl(body.Url, false)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid URL: %v", err)
	}

	identity := new(protocol.ADI)
	identity.Url = body.Url
	accounts := []protocol.Account{identity}

	// Create a new key book
	if body.KeyBookUrl != nil {
		// Verify the user provided a first key
		if len(body.KeyHash) == 0 {
			return nil, errors.BadRequest.WithFormat("missing PublicKey which is required when creating a new KeyBook/KeyPage pair")
		}

		// Verify the URL is ok
		err = validateKeyBookUrl(body.KeyBookUrl, body.Url)
		if err != nil {
			return nil, err
		}

		// Add it to the authority set
		identity.AddAuthority(body.KeyBookUrl)

		// Create the book
		book := new(protocol.KeyBook)
		book.Url = body.KeyBookUrl
		book.PageCount = 1
		book.AddAuthority(body.KeyBookUrl)
		accounts = append(accounts, book)
		if len(body.KeyHash) != 32 {
			return nil, errors.BadRequest.WithFormat("invalid Key Hash: length must be equal to 32 bytes")
		}

		// Create the page
		page := new(protocol.KeyPage)
		page.Version = 1
		page.Url = protocol.FormatKeyPageUrl(body.KeyBookUrl, 0)
		page.AcceptThreshold = 1 // Require one signature from the Key Page
		keySpec := new(protocol.KeySpec)
		keySpec.PublicKeyHash = body.KeyHash
		page.AddKeySpec(keySpec)
		accounts = append(accounts, page)
	}

	// Add additional authorities or inherit
	err = x.setAuth(batch, ctx, identity)
	if err != nil {
		return nil, err
	}

	// Produce a SyntheticCreateIdentity
	ctx.didProduce(body.Url, &messaging.UserTransaction{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: body.Url,
			},
			Body: &protocol.SyntheticCreateIdentity{Accounts: accounts},
		},
	})

	return nil, nil
}

func validateKeyBookUrl(bookUrl *url.URL, adiUrl *url.URL) error {
	err := protocol.IsValidAdiUrl(bookUrl, false)
	if err != nil {
		return errors.BadRequest.WithFormat("invalid KeyBook URL %s: %v", bookUrl.String(), err)
	}
	parent, ok := bookUrl.Parent()
	if !ok {
		return errors.BadRequest.WithFormat("invalid URL %s, the KeyBook URL must be adi_path/KeyBook", bookUrl)
	}
	if !parent.Equal(adiUrl) {
		return errors.BadRequest.WithFormat("KeyBook %s must be a direct child of its ADI %s", bookUrl.String(), adiUrl.String())
	}
	return nil
}
