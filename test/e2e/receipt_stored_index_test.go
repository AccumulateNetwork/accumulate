// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4263: the two index positions a receipt needs are recorded when the entry
// is written (TransactionChainEntry.ChainIndex and .AnchorIndex), so the
// receipt reads them instead of searching the index chains for them. The
// receipt that results must be the one the search would have produced.
func TestReceiptFromStoredIndexMatchesTheSearch(t *testing.T) {
	var timestamp uint64
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// A run of transactions, so entries sit at a range of depths
	for i := 0; i < 8; i++ {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(aliceUrl).
				BurnTokens(1, 0).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice))
		sim.StepUntil(Txn(st.TxID).Succeeds())
	}
	sim.StepN(25)

	part, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	net := config.NetworkUrl{URL: protocol.PartitionUrl(part)}

	var checked int
	View(t, sim.Database(part), func(batch *database.Batch) {
		c, err := batch.Account(aliceUrl).ChainByName("main")
		require.NoError(t, err)
		chain, err := c.Get()
		require.NoError(t, err)

		for i := int64(0); i < chain.Height(); i++ {
			// What the stored positions produce
			gotEntry, gotRootIdx, gotReceipt, err := indexing.ReceiptForChainIndex(net, batch, c, i, nil)
			require.NoError(t, err, "entry %d", i)

			// What a search produces, computed here so the comparison is
			// against the original behaviour and not against itself
			wantEntry, wantRootIdx, wantReceipt := receiptBySearch(t, net, batch, c, i)

			require.Equal(t, wantRootIdx, gotRootIdx, "entry %d: root index position", i)
			require.Equal(t, wantEntry.Source, gotEntry.Source, "entry %d: root entry source", i)
			require.Equal(t, wantEntry.BlockIndex, gotEntry.BlockIndex, "entry %d: block", i)
			require.Equal(t, wantReceipt.Anchor, gotReceipt.Anchor, "entry %d: anchor", i)
			require.Equal(t, wantReceipt.Start, gotReceipt.Start, "entry %d: start", i)
			require.Equal(t, len(wantReceipt.Entries), len(gotReceipt.Entries), "entry %d: proof length", i)
			require.True(t, gotReceipt.Validate(nil), "entry %d: the receipt must verify", i)
			checked++
		}
	})
	require.NotZero(t, checked, "no chain entries were checked")
	t.Logf("checked %d entries: the stored positions give the search's receipt", checked)
}

// receiptBySearch reproduces the pre-#4263 path: locate both index entries by
// searching, then build the receipt from them.
func receiptBySearch(t *testing.T, net config.NetworkUrl, batch *database.Batch, c *database.Chain2, index int64) (*protocol.IndexEntry, uint64, *merkle.Receipt) {
	t.Helper()
	indexChain, err := c.Index().Get()
	require.NoError(t, err)
	_, entry, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainBySource(uint64(index)))
	require.NoError(t, err)

	rootIndexChain, err := batch.Account(net.Ledger()).RootChain().Index().Get()
	require.NoError(t, err)
	rootIdx, rootEntry, err := indexing.SearchIndexChain(rootIndexChain, uint64(rootIndexChain.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainBySource(entry.Anchor))
	require.NoError(t, err)

	chain, err := c.Get()
	require.NoError(t, err)
	accountReceipt, err := chain.Receipt(index, int64(entry.Source))
	require.NoError(t, err)
	rootChain, err := batch.Account(net.Ledger()).RootChain().Get()
	require.NoError(t, err)
	rootReceipt, err := rootChain.Receipt(int64(entry.Anchor), int64(rootEntry.Source))
	require.NoError(t, err)
	r, err := accountReceipt.Combine(rootReceipt)
	require.NoError(t, err)
	return rootEntry, rootIdx, r
}
