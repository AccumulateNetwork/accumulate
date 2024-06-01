// Copyright 2024 The Accumulate Authors
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
	// TODO Deduplicate. Duplicating critical code like this is not good. This
	// is mostly copied from block.TransactionContext.userTransactionIsReady.

	// For each authority
	notReady := map[[32]byte]struct{}{}
	for _, authority := range a {
		// Check if any signer has reached its threshold
		ok, vote, err := delegate.AuthorityDidVote(batch, transaction, authority)
		if err != nil {
			return false, false, errors.UnknownError.Wrap(err)
		}

		// Did the authority vote?
		if !ok {
			notReady[authority.AccountID32()] = struct{}{}
			continue
		}

		// Any vote that is not accept is counted as reject. Since a transaction
		// is only executed once _all_ authorities accept, a single authority
		// rejecting or abstaining is sufficient to block the transaction.
		if vote != protocol.VoteTypeAccept {
			return false, false, errors.Rejected
		}
	}

	// The transaction is only ready if all authorities have voted
	if len(notReady) > 0 {
		return false, false, nil
	}

	// Fallback
	return false, true, nil
}
