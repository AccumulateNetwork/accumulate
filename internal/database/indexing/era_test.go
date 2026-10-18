// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An account that lived on Apollo before the reorg and on Cyclops since. Its
// first two main chain entries were anchored into Apollo's root chain, which
// only the era holds; its third into Cyclops's.
type reorgFixture struct {
	live, era *database.Batch
	account   *url.URL
	old       []*protocol.IndexEntry // the two pre-reorg index entries
}

func hash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func appendRoot(t *testing.T, ledger *database.Account, anchor []byte, block uint64) uint64 {
	t.Helper()
	root := ledger.RootChain()
	require.NoError(t, root.Inner().AddEntry(anchor, false))
	chain, err := root.Get()
	require.NoError(t, err)
	pos := uint64(chain.Height() - 1)
	entry := &protocol.IndexEntry{Source: pos, BlockIndex: block}
	b, err := entry.MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, root.Index().Inner().AddEntry(b, false))
	return pos
}

func addIndexed(t *testing.T, acct *database.Account, ledger *database.Account, entry []byte, block uint64) *protocol.IndexEntry {
	t.Helper()
	main := acct.MainChain()
	require.NoError(t, main.Inner().AddEntry(entry, false))
	chain, err := main.Get()
	require.NoError(t, err)
	source := uint64(chain.Height() - 1)
	anchor, err := chain.AnchorAt(source)
	require.NoError(t, err)
	pos := appendRoot(t, ledger, anchor, block)
	ie := &protocol.IndexEntry{Source: source, Anchor: pos, BlockIndex: block}
	b, err := ie.MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, main.Index().Inner().AddEntry(b, false))
	return ie
}

func newReorgFixture(t *testing.T) *reorgFixture {
	t.Helper()
	f := &reorgFixture{account: url.MustParse("acc://moved.acme/tokens")}
	apollo := protocol.PartitionUrl("Apollo").JoinPath(protocol.Ledger)
	cyclops := protocol.PartitionUrl("Cyclops").JoinPath(protocol.Ledger)

	// Before the reorg, on Apollo: the account's chain and Apollo's root
	// chain, as the era holds them
	old := database.OpenInMemory(nil).Begin(true)
	t.Cleanup(old.Discard)
	for i := 0; i < 5; i++ { // Apollo's root chain had other anchors first
		appendRoot(t, old.Account(apollo), hash(fmt.Sprint("other", i)), uint64(1000+i))
	}
	f.old = append(f.old, addIndexed(t, old.Account(f.account), old.Account(apollo), hash("entry 0"), 6552823))
	f.old = append(f.old, addIndexed(t, old.Account(f.account), old.Account(apollo), hash("entry 1"), 6552900))

	// The routing table in force then
	table := &protocol.RoutingTable{
		Overrides: []protocol.RouteOverride{{Account: f.account.RootIdentity(), Partition: "Apollo"}},
		Routes:    []protocol.Route{{Length: 1, Value: 0, Partition: "Yutu"}, {Length: 1, Value: 1, Partition: "Chandrayaan"}},
	}
	tb, err := table.MarshalBinary()
	require.NoError(t, err)
	routing := protocol.DnUrl().JoinPath(protocol.Routing)
	require.NoError(t, old.Account(routing).Main().Put(&protocol.DataAccount{Url: routing, Entry: &protocol.AccumulateDataEntry{Data: [][]byte{tb}}}))
	f.era = old

	// Since the reorg, on Cyclops: the account's chain kept whole, its
	// index chain too, and a third entry anchored into Cyclops's root chain
	live := database.OpenInMemory(nil).Begin(true)
	t.Cleanup(live.Discard)
	main := live.Account(f.account).MainChain()
	require.NoError(t, main.Inner().AddEntry(hash("entry 0"), false))
	require.NoError(t, main.Inner().AddEntry(hash("entry 1"), false))
	for _, ie := range f.old {
		b, err := ie.MarshalBinary()
		require.NoError(t, err)
		require.NoError(t, main.Index().Inner().AddEntry(b, false))
	}
	addIndexed(t, live.Account(f.account), live.Account(cyclops), hash("entry 2"), 1)
	f.live = live
	return f
}

// #4567: an entry written before the reorg proves against the routed
// partition's pre-reorg root chain, through the era; one written since
// proves against the live root chain; and without an era the old entry
// cannot be proved at all.
func TestChainReceiptThroughTheEra(t *testing.T) {
	f := newReorgFixture(t)
	cyclops := config.NetworkUrl{URL: protocol.PartitionUrl("Cyclops")}
	chain := f.live.Account(f.account).MainChain()

	era := &indexing.Era{Batch: f.era}
	table, err := era.RouteTable()
	require.NoError(t, err)
	require.Equal(t, "Apollo", table.Overrides[0].Partition)
	era.Route = func(*url.URL) (string, error) { return "Apollo", nil }

	t.Run("before the reorg", func(t *testing.T) {
		r, err := indexing.ChainReceiptFor(cyclops, f.live, chain, 0, nil, era)
		require.NoError(t, err)
		require.True(t, r.Era)
		require.Equal(t, "Apollo", r.Partition)
		require.EqualValues(t, 6552823, r.RootEntry.BlockIndex)
		require.True(t, r.Receipt.Validate(nil))
		require.Equal(t, hash("entry 0"), r.Receipt.Start)

		// The receipt ends at Apollo's root chain state at that height
		root, err := f.era.Account(protocol.PartitionUrl("Apollo").JoinPath(protocol.Ledger)).RootChain().Get()
		require.NoError(t, err)
		anchor, err := root.AnchorAt(f.old[0].Anchor)
		require.NoError(t, err)
		require.Equal(t, anchor, r.Receipt.Anchor)
	})

	t.Run("since the reorg", func(t *testing.T) {
		r, err := indexing.ChainReceiptFor(cyclops, f.live, chain, 2, nil, era)
		require.NoError(t, err)
		require.False(t, r.Era)
		require.Equal(t, "Cyclops", r.Partition)
		require.EqualValues(t, 1, r.RootEntry.BlockIndex)
		require.True(t, r.Receipt.Validate(nil))
	})

	t.Run("without an era", func(t *testing.T) {
		_, err := indexing.ChainReceiptFor(cyclops, f.live, chain, 0, nil, nil)
		require.Error(t, err)
		_, _, _, err = indexing.ReceiptForChainIndex(cyclops, f.live, chain, 0, nil)
		require.Error(t, err)
	})
}

// The restore's block-1 entry of some Directory accounts anchors a state the
// chain never had, so it proves against no root chain. The element it
// covers is proved through the next entry of the live era.
func TestChainReceiptSkipsAnUnprovableIndexEntry(t *testing.T) {
	cyclops := config.NetworkUrl{URL: protocol.PartitionUrl("Cyclops")}
	ledger := cyclops.Ledger()
	account := url.MustParse("acc://restored.acme/book")
	live := database.OpenInMemory(nil).Begin(true)
	defer live.Discard()

	// Entry 0 is anchored by an index entry whose anchor is a root chain
	// element that is not the chain's state (the anomaly); entry 1 is
	// anchored correctly, and covers entry 0 too
	main := live.Account(account).MainChain()
	require.NoError(t, main.Inner().AddEntry(hash("genesis"), false))
	pos := appendRoot(t, live.Account(ledger), hash("a state the chain never had"), 1)
	ie := &protocol.IndexEntry{Source: 0, Anchor: pos, BlockIndex: 1}
	b, err := ie.MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, main.Index().Inner().AddEntry(b, false))
	addIndexed(t, live.Account(account), live.Account(ledger), hash("later"), 2)

	era := &indexing.Era{Batch: live, Route: func(*url.URL) (string, error) { return "Directory", nil }}
	r, err := indexing.ChainReceiptFor(cyclops, live, main, 0, nil, era)
	require.NoError(t, err)
	require.False(t, r.Era)
	require.EqualValues(t, 2, r.RootEntry.BlockIndex, "proved through the next entry")
	require.True(t, r.Receipt.Validate(nil))
	require.Equal(t, hash("genesis"), r.Receipt.Start)
}
