// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// peerSources is one peer: a real querier over a real store, reached the way
// the join reaches a peer. Its answers are the API's, receipts and all.
type peerSources struct {
	partition *url.URL
	querier   api.Querier
}

func (p *peerSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{api.Querier2{Querier: p.querier}}, p.partition, nil
}

func (p *peerSources) Querier(*url.URL) api.Querier { return p.querier }

// noAnchors is a Directory that has executed no anchor, so every block a peer
// serves at is one AnchoredRoot answers ErrNotAnchored for.
type noAnchors struct{}

func (noAnchors) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	return nil, errors.NotFound.With("the directory has executed no anchors")
}

// putLedger writes a partition's system ledger at a block.
func putLedger(t *testing.T, db *database.Database, partition *url.URL, block uint64) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = partition.JoinPath(protocol.Ledger)
	ledger.Index = block
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
}

// TestLocalBlock_IsTheExecutorsBlockAndNotThePulledLedger.
//
// `<partition>/ledger` is an account, and it is one of the accounts the pull
// overwrites with the peer's. So a join that asks the store where this node
// stands is asking a question the store stopped being able to answer the
// moment the pull started. Measured on the live twelve-node network of
// 2026-09-18: the joining node's own store answered 929 and the peer 946,
// while its executor was at block 76 (#4295).
//
// What that costs is the whole of the primary path: peer - local is 17
// instead of 853, so the span never exceeds MaxLedgerSpan, s.wide is never
// set, the BPT page diff never runs as the primary, and the block-ledger walk
// covers seventeen blocks of a node that is eight hundred and fifty behind.
func TestLocalBlock_IsTheExecutorsBlockAndNotThePulledLedger(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")

	// This node executed block 76 and stopped: a joining node collects
	// committed blocks and executes none.
	local := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = local.Close() })
	putLedger(t, local, here, 76)

	// A peer at block 946.
	peer := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = peer.Close() })
	putLedger(t, peer, here, 946)
	srcs := &peerSources{
		partition: here,
		querier:   apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"}),
	}

	values, _ := genesisValues(t, 4)
	putNetwork(t, local, here, values)

	s, err := NewState(StateOptions{Partition: here, Database: local, Sources: srcs})
	require.NoError(t, err)

	r, err := s.localBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(76), r, "this node's executor is at 76")

	// Now the pull writes the peer's ledger into this node's store, which is
	// exactly what pulling that account means.
	putLedger(t, local, here, 946)

	r, err = s.localBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(76), r,
		"the join must read its own executor's block, not the ledger the pull overwrote")

	// And the round that follows knows it is 870 blocks behind, so it goes
	// wide and the page diff runs as the primary.
	_, err = s.changedAccounts(ctx)
	require.NoError(t, err)
	require.True(t, s.wide,
		"a node 870 blocks behind must take the page diff, not walk seventeen blocks")
}
