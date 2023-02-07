// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateKeyPage struct{}

func (CreateKeyPage) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyPage }

func (CreateKeyPage) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (CreateKeyPage{}).Validate(st, tx)
}

func (CreateKeyPage) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	var book *protocol.KeyBook
	switch origin := st.Origin.(type) {
	case *protocol.KeyBook:
		book = origin
	default:
		return nil, fmt.Errorf("invalid principal: want account type %v, got %v", protocol.AccountTypeKeyBook, origin.Type())
	}

	body, ok := tx.Transaction.Body.(*protocol.CreateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyPage), tx.Transaction.Body)
	}

	if len(body.Keys) == 0 {
		return nil, fmt.Errorf("cannot create empty sig spec")
	}

	//check for duplicate entries
	uniqueKeys := make(map[string]bool, len(body.Keys))
	for _, key := range body.Keys {
		switch len(key.KeyHash) {
		case 0:
			continue
		case 32:
			if uniqueKeys[string(key.KeyHash)] {
				return nil, fmt.Errorf("duplicate keys: signing keys of a keypage must be unique")
			}
			uniqueKeys[string(key.KeyHash)] = true
		default:
			return nil, errors.BadRequest.WithFormat("public key hash length is invalid")
		}
	}

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(book.Url, book.PageCount)
	page.AcceptThreshold = 1 // Require one signature from the Key Page
	book.PageCount++

	if book.PageCount > st.Globals.Globals.Limits.BookPages {
		return nil, errors.BadRequest.WithFormat("book will have too many pages")
	}
	if len(body.Keys) > int(st.Globals.Globals.Limits.PageEntries) {
		return nil, errors.BadRequest.WithFormat("page will have too many entries")
	}

	for _, sig := range body.Keys {
		ss := new(protocol.KeySpec)
		ss.PublicKeyHash = sig.KeyHash
		page.AddKeySpec(ss)
	}

	err := st.Update(book)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %w", book.Url, err)
	}

	err = st.Create(page)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", page.Url, err)
	}

	return nil, nil
}
