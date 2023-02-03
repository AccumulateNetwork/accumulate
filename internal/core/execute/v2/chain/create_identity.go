// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateIdentity struct{}

var _ SignerValidator = (*CreateIdentity)(nil)
var _ PrincipalValidator = (*CreateIdentity)(nil)

func (CreateIdentity) Type() protocol.TransactionType { return protocol.TransactionTypeCreateIdentity }

func (CreateIdentity) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return false, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateIdentity), transaction.Body)
	}

	// Anyone is allowed to sign for a root identity
	if body.Url.IsRootIdentity() {
		return false, nil
	}

	// Check additional authorities
	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, signer, md)
}

func (CreateIdentity) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return false, false, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateIdentity), transaction.Body)
	}

	// Check additional authorities
	ready, fallback, err = additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
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

func (CreateIdentity) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateIdentity{}).Validate(st, tx)
}

func (CreateIdentity) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateIdentity)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateIdentity), tx.Transaction.Body)
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
		err := checkCreateAdiAccount(st, body.Url)
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
	err = st.SetAuth(identity, body.Authorities)
	if err != nil {
		return nil, err
	}

	// If the ADI is remote, use a synthetic transaction
	if !tx.Transaction.Header.Principal.LocalTo(body.Url) {
		st.Submit(body.Url, &protocol.SyntheticCreateIdentity{Accounts: accounts})
		return nil, nil
	}

	// If the ADI is local, create it directly
	err = st.Create(accounts...)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("failed to create %v: %v", body.Url, err)
	}

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
