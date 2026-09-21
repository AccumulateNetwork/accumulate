// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
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
	_, err = s.changedAccounts(ctx, 946)
	require.NoError(t, err)
	require.True(t, s.wide,
		"a node 870 blocks behind must take the page diff, not walk seventeen blocks")
}

// TestFetch_AnAccountThatCannotSettleIsRefusedAndNotHeld.
//
// THERE IS NO SETTLE WINDOW ANY MORE, and that is #4362's centre. The pull
// used to ask a peer for whatever block it was on and then HOLD what came
// back for maxSettleRounds rounds, hoping the Directory would anchor that
// block. The Directory anchors roughly one block in six of a BVN's, so five
// batches in six waited for an anchor that was never coming and were thrown
// away -- and an account that changes every block could never settle at all.
//
// A pass asks for a block whose root it already holds and has verified, so
// there is nothing to wait for: an account either verifies against that root
// now, or it is refused and asked for again next round, at the same block.
// Nothing is held across rounds, so nothing is discarded after a wait and
// nothing is re-fetched into a second batch whose commits race (#4352,
// #4353).
func TestFetch_AnAccountThatCannotSettleIsRefusedAndNotHeld(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	account := protocol.AccountUrl("alice", "tokens")

	peer := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = peer.Close() })
	putLedger(t, peer, here, 946)
	writeTokenAccount(t, peer, account)

	local := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = local.Close() })

	log := new(syncBuffer)
	s := &PulledState{
		partition: here,
		db:        local,
		sources: &peerSources{
			partition: here,
			querier:   apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"}),
		},
		log: slog.New(slog.NewTextHandler(log, nil)),
	}
	// A Directory that has anchored nothing, so nothing the peer serves can
	// verify.
	values, _ := genesisValues(t, 4)
	s.anchors = noAnchorSource(t, here, values)

	pulled, refused := s.fetch(ctx, &syncPass{block: 946}, []*url.URL{account})
	require.Zero(t, pulled, "nothing verifies against a root nobody signed")
	require.Equal(t, []*url.URL{account}, refused,
		"it is refused and asked for again, not held: an account held across "+
			"rounds is an account re-fetched at a different block, and the two "+
			"commits race (#4353)")
	require.Contains(t, log.String(), "could not be pulled",
		"a refusal says so; the hold this replaces was silent (#4295)")
}

// writeTokenAccount gives the peer something to serve, with the root index
// entry a state receipt is reported at.
func writeTokenAccount(t *testing.T, db *database.Database, u *url.URL) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u}))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	batch2 := db.Begin(true)
	defer batch2.Discard()
	ledger := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	root, err := batch2.Account(ledger).RootChain().Index().Get()
	require.NoError(t, err)
	data, err := (&protocol.IndexEntry{BlockIndex: 946}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, root.AddEntry(data, false))
	require.NoError(t, batch2.UpdateBPT())
	require.NoError(t, batch2.Commit())
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
