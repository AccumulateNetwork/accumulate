// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticCreateIdentity struct{}

var _ PrincipalValidator = (*SyntheticCreateIdentity)(nil)

func (SyntheticCreateIdentity) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticCreateIdentity
}

func (SyntheticCreateIdentity) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// SyntheticCreateChain can create accounts
	return true
}

func (SyntheticCreateIdentity) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticCreateIdentity{}).Validate(st, tx)
}

func (SyntheticCreateIdentity) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticCreateIdentity)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticCreateIdentity), tx.Transaction.Body)
	}

	if body.Cause == nil {
		return nil, fmt.Errorf("cause is missing")
	}

	// Do basic validation and add everything to the state manager
	err := st.Create(body.Accounts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create %v: %w", body.Accounts[0].GetUrl(), err)
	}

	// Verify everything is sane
	for _, account := range body.Accounts {
		u := account.GetUrl()
		record, err := st.LoadUrl(u)
		if err != nil {
			// This really shouldn't happen, but don't panic
			return nil, fmt.Errorf("internal error: failed to fetch pending record")
		}

		// Check the identity
		switch record.Type() {
		case protocol.AccountTypeIdentity:
		default:
			// Make sure the ADI actually exists
			_, err = st.LoadUrl(u.Identity())
			if errors.Is(err, storage.ErrNotFound) {
				return nil, fmt.Errorf("missing identity for %s", u.String())
			} else if err != nil {
				return nil, fmt.Errorf("error fetching %q: %v", u.String(), err)
			}

		}

		// Check the key book
		fullAccount, ok := record.(protocol.FullAccount)
		if !ok {
			continue
		}

		auth := fullAccount.GetAuth()
		if len(auth.Authorities) == 0 {
			return nil, fmt.Errorf("%q does not specify a key book", u)
		}

		// Make sure the key book actually exists
		for _, auth := range auth.Authorities {
			if !record.GetUrl().LocalTo(auth.Url) {
				continue
			}

			var book *protocol.KeyBook
			err = st.LoadUrlAs(auth.Url, &book)
			if err != nil {
				return nil, fmt.Errorf("invalid key book %q for %q: %v", auth.Url, u, err)
			}
		}
	}

	return nil, nil
}
