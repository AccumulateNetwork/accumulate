// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// An account named with user info, a port, a query or a fragment has the same
// identity as one named without them (url.AccountID) and compares unequal to
// it; CheckAccountUrls refuses such a name wherever it appears.
func TestCheckAccountUrls(t *testing.T) {
	send := func(principal, to string) *Transaction {
		txn := new(Transaction)
		txn.Header.Principal = url.MustParse(principal)
		body := new(SendTokens)
		body.AddRecipient(url.MustParse(to), big.NewInt(1))
		txn.Body = body
		return txn
	}

	require.NoError(t, CheckAccountUrls(send("alice.acme/tokens", "bob.acme/tokens")))

	for _, bad := range []string{
		"acc://user@alice.acme/tokens",
		"acc://alice.acme:1/tokens",
		"acc://alice.acme:/tokens",
		"acc://alice.acme/tokens?x=1",
		"acc://alice.acme/tokens#x",
	} {
		t.Run(bad, func(t *testing.T) {
			require.ErrorIs(t, CheckAccountUrls(send(bad, "bob.acme/tokens")), errors.BadRequest, "in the header")
			require.ErrorIs(t, CheckAccountUrls(send("alice.acme/tokens", bad)), errors.BadRequest, "in the body")
			sig := &DelegatedSignature{
				Signature: &ED25519Signature{Signer: url.MustParse("alice.acme/book/1")},
				Delegator: url.MustParse(bad),
			}
			require.ErrorIs(t, CheckAccountUrls(sig), errors.BadRequest, "in a signature")
		})
	}
}
