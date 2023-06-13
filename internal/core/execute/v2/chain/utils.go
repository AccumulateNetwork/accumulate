// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type additionalAuthorities []*url.URL

// AuthorityIsAccepted authorizes a signer if it belongs to an additional
// authority. Otherwise, it falls back to the default.
func (a additionalAuthorities) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	for _, authority := range a {
		// If we are adding a book, that book is authorized to sign the
		// transaction
		if authority.Equal(sig.Authority) {
			return false, nil
		}
	}

	// Fallback to general authorization
	return true, nil
}

// TransactionIsReady verifies that each additional authority is satisfied.
func (a additionalAuthorities) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	for _, authority := range a {
		ok, err := delegate.AuthorityDidVote(batch, transaction, authority)
		if !ok || err != nil {
			return false, false, err
		}
	}

	// Fallback to general authorization
	return false, true, nil
}
