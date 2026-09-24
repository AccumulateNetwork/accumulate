// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestALeafWithNoBodyIsServedWithItsReceipt: a peer whose state tree holds a
// leaf for an account with no main state answers a request for a current
// receipt with no body and the receipt for that leaf (#4397), and answers
// NotFound only where it holds no leaf.
//
// Both families the load generator's failed work leaves in the tree are built
// here the way the executor builds them: a principal that does not exist,
// given a signature chain entry by RecordHistory (sig_authority.go), and a
// failed deposit's target, an empty account's leaf with no chains at all.
func TestALeafWithNoBodyIsServedWithItsReceipt(t *testing.T) {
	const partitionID = "MainlessLeaf"
	part := protocol.PartitionUrl(partitionID)
	sysLedger := part.JoinPath(protocol.Ledger)
	ghost := url.MustParse("alice/ghostdata1")
	void := url.MustParse("void-9aac09e22e861b50/tokens")
	absent := url.MustParse("nobody/tokens")

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	ledger := new(protocol.SystemLedger)
	ledger.Url = sysLedger
	ledger.Index = 9
	require.NoError(t, batch.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, batch.Account(ghost).SignatureChain().Inner().AddEntry(make([]byte, 32), false))
	require.NoError(t, batch.Account(void).MarkDirty())
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	batch = db.Begin(true)
	root, err := batch.Account(sysLedger).RootChain().Index().Get()
	require.NoError(t, err)
	data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, root.AddEntry(data, false))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	// What the tree holds: a leaf for both, and a body for neither.
	var bptRoot [32]byte
	batch = db.Begin(false)
	for _, u := range []*url.URL{ghost, void} {
		_, err := batch.BPT().Get(batch.Account(u).Key())
		require.NoError(t, err, "%v has no leaf", u)
		_, err = batch.Account(u).Main().Get()
		require.ErrorIs(t, err, errors.NotFound, "%v has a body", u)
	}
	bptRoot, err = batch.GetBptRootHash()
	require.NoError(t, err)
	batch.Discard()

	q := api.Querier2{Querier: NewQuerier(QuerierParams{Database: db, Partition: partitionID})}
	ctx := context.Background()
	withReceipt := &api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}}

	for _, c := range []struct {
		name   string
		url    *url.URL
		chains []string
	}{
		// The executor also indexes the chain (signature-index); a bare append
		// does not, and the chain it did append is what is being served.
		{"signature chains, no body", ghost, []string{"signature"}},
		{"empty account, no chains", void, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec, err := q.QueryAccount(ctx, c.url, withReceipt)
			require.NoError(t, err, "a leaf the tree holds was answered as absent")
			require.Nil(t, rec.Account, "a body was invented for an account that has none")
			require.NotNil(t, rec.Receipt, "no receipt was served")
			require.True(t, rec.Receipt.Receipt.Validate(nil))
			require.Equal(t, make([]byte, 32), rec.Receipt.Receipt.Start,
				"the receipt must start at the zero hash a missing body hashes to")
			require.Equal(t, bptRoot[:], rec.Receipt.Receipt.Anchor,
				"the receipt must end at the peer's BPT root")

			// The rest of the leaf is served by the query that serves it for
			// any account.
			chains, err := q.QueryAccountChains(ctx, c.url, &api.ChainQuery{})
			require.NoError(t, err)
			var names []string
			for _, r := range chains.Records {
				names = append(names, r.Name)
			}
			require.ElementsMatch(t, c.chains, names)

			// A reader that did not ask for a receipt sees what it saw before.
			_, err = q.QueryAccount(ctx, c.url, nil)
			require.ErrorIs(t, err, errors.NotFound)
		})
	}

	t.Run("no leaf", func(t *testing.T) {
		_, err := q.QueryAccount(ctx, absent, withReceipt)
		require.ErrorIs(t, err, errors.NotFound, "an account the tree has no leaf for must be NotFound")
	})
}
