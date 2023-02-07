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

type CreateLiteTokenAccount struct{}

var _ SignerValidator = (*CreateLiteTokenAccount)(nil)
var _ PrincipalValidator = (*CreateLiteTokenAccount)(nil)

func (CreateLiteTokenAccount) Type() protocol.TransactionType {
	return protocol.TransactionTypeCreateLiteTokenAccount
}

func (CreateLiteTokenAccount) validate(transaction *protocol.Transaction) (*url.URL, error) {
	_, ok := transaction.Body.(*protocol.CreateLiteTokenAccount)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.CreateLiteTokenAccount), transaction.Body)
	}
	key, tok, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
	if key == nil {
		return nil, errors.BadRequest.WithFormat("invalid lite token account URL: %v", transaction.Header.Principal)
	}
	return tok, nil
}

func (CreateLiteTokenAccount) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	_, err = CreateLiteTokenAccount{}.validate(transaction)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	// Anyone is allowed to create a lite token account
	return false, nil
}

func (CreateLiteTokenAccount) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	_, err = CreateLiteTokenAccount{}.validate(transaction)
	if err != nil {
		return false, false, errors.UnknownError.Wrap(err)
	}

	// Anyone is allowed to create a lite token account
	return true, false, nil
}

func (CreateLiteTokenAccount) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// The lite token account should be missing
	return true
}

func (CreateLiteTokenAccount) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return CreateLiteTokenAccount{}.Validate(st, tx)
}

func (CreateLiteTokenAccount) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	tok, err := CreateLiteTokenAccount{}.validate(tx.Transaction)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Only ACME for now
	if !protocol.AcmeUrl().Equal(tok) {
		return nil, errors.BadRequest.WithFormat("creating non-ACME lite token accounts is not supported")
	}

	// Will fail if the account already exists. DO NOT set any other properties
	// like the lock, because this transaction can be executed without the key
	// holder's consent.
	account := new(protocol.LiteTokenAccount)
	account.Url = tx.Transaction.Header.Principal
	account.TokenUrl = tok
	err = st.Create(account)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create lite token account: %w", err)
	}

	var root *protocol.LiteIdentity
	err = st.batch.Account(account.Url.RootIdentity()).Main().GetAs(&root)
	switch {
	case err == nil:
		// Lite identity exists, nothing to do
		return nil, nil

	case !errors.Is(err, errors.NotFound):
		// Unknown error
		return nil, errors.UnknownError.WithFormat("load lite identity: %w", err)
	}

	root = new(protocol.LiteIdentity)
	root.Url = account.Url.RootIdentity()
	err = st.Create(root)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create lite identity: %w", err)
	}

	return nil, nil
}
