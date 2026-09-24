// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"crypto/sha256"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// The state tree holds no leaf for an account that holds nothing, and hides
// no change to one that holds something (executor spec, invariant 13; #4437).
func TestAnAccountThatHoldsNothingGetsNoLeafAndOneThatHoldsSomethingIsReported(t *testing.T) {
	db := OpenInMemory(nil)
	blank := url.MustParse("void/tokens")
	stateless := url.MustParse("ghost/data")
	txid := sha256.Sum256([]byte("a transaction for a principal that does not exist"))

	before := testutil.ToFloat64(mStatelessAccountLeaves)
	b := db.Begin(true)
	// Bookkeeping only: the account is dirty and holds nothing.
	require.NoError(t, b.Account(blank).Transaction(txid).Votes().Put(nil))
	// A chain on an account with no main state: something the tree must see.
	require.NoError(t, b.Account(stateless).SignatureChain().Inner().AddEntry(txid[:], false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	b = db.Begin(false)
	defer b.Discard()
	_, err := b.BPT().Get(b.Account(blank).Key())
	require.ErrorIs(t, err, errors.NotFound, "an account that holds nothing got a leaf")
	_, err = b.BPT().Get(b.Account(stateless).Key())
	require.NoError(t, err, "a change to an account with no main state was hidden from the tree")
	require.Equal(t, before+1, testutil.ToFloat64(mStatelessAccountLeaves),
		"a leaf for an account with no main state that holds a chain was not reported")
}
