// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateKeyBook struct{}

var _ SignerValidator = (*CreateKeyBook)(nil)

func (CreateKeyBook) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyBook }

func (CreateKeyBook) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).SignerIsAuthorized(delegate, batch, transaction, signer, md)
}

func (CreateKeyBook) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	body, ok := transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return false, false, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), transaction.Body)
	}

	return additionalAuthorities(body.Authorities).TransactionIsReady(delegate, batch, transaction, status)
}

func (CreateKeyBook) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateKeyBook{}).Validate(st, tx)
}

func (CreateKeyBook) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), tx.Transaction.Body)
	}

	if body.Url == nil {
		return nil, errors.BadRequest.WithFormat("account URL is missing")
	}

	for _, u := range body.Authorities {
		if u == nil {
			return nil, errors.BadRequest.WithFormat("authority URL is nil")
		}
	}

	err := checkCreateAdiAccount(st, body.Url)
	if err != nil {
		return nil, err
	}

	switch len(body.PublicKeyHash) {
	case 0:
		return nil, errors.BadRequest.WithFormat("public key hash is missing")
	case 32:
		// Ok
	default:
		return nil, errors.BadRequest.WithFormat("public key hash length is invalid")
	}

	book := new(protocol.KeyBook)
	book.Url = body.Url
	book.AddAuthority(body.Url)
	book.PageCount = 1

	err = st.SetAuth(book, body.Authorities)
	if err != nil {
		return nil, err
	}

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(body.Url, 0)

	key := new(protocol.KeySpec)
	key.PublicKeyHash = body.PublicKeyHash
	page.Keys = []*protocol.KeySpec{key}

	err = st.Create(book, page)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", book.Url, err)
	}
	return nil, nil
}
