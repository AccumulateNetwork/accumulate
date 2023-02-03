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

// SignerIsAuthorized authorizes a signer if it belongs to an additional
// authority. Otherwise, it falls back to the default.
func (a additionalAuthorities) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, _ SignatureValidationMetadata) (fallback bool, err error) {
	signerBook, _, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		// Fallback to general authorization
		return true, nil
	}

	for _, authority := range a {
		// If we are adding a book, that book is authorized to sign the
		// transaction
		if !authority.Equal(signerBook) {
			continue
		}

		return false, delegate.SignerIsAuthorized(batch, transaction, signer, false)
	}

	// Fallback to general authorization
	return true, nil
}

// TransactionIsReady verifies that each additional authority is satisfied.
func (a additionalAuthorities) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	for _, authority := range a {
		ok, err := delegate.AuthorityIsSatisfied(batch, transaction, status, authority)
		if !ok || err != nil {
			return false, false, err
		}
	}

	// Fallback to general authorization
	return false, true, nil
}
