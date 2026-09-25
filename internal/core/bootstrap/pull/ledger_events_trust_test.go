// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// The trust questions of #4399's scheduled events, from the review of
// issue-4397-mainless-leaf (#4397 note_3896123320, F1, F2, F5).

package pull

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// rewriteEvents lets a peer serve a different LedgerEvents than it holds.
type rewriteEvents struct {
	Source
	fn func(*api.LedgerEvents)
}

func (r rewriteEvents) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	rec, err := r.Source.QueryAccount(ctx, u, q)
	if err != nil {
		return nil, err
	}
	if rec.Leaf != nil && rec.Leaf.Events != nil {
		r.fn(rec.Leaf.Events)
	}
	return rec, nil
}

func ledgerWithOneVote(t *testing.T) (src *database.Database, root [32]byte, block uint64, part, sysLedger *url.URL, vote *protocol.AuthoritySignature, partitionID string) {
	partitionID = "EventsTrust"
	part = protocol.PartitionUrl(partitionID)
	sysLedger = part.JoinPath(protocol.Ledger)
	pending := protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{1})
	vote = &protocol.AuthoritySignature{
		Origin:    protocol.AccountUrl("alice", "book", "1"),
		Authority: protocol.AccountUrl("alice", "book"),
		Vote:      protocol.VoteTypeAccept,
		TxID:      pending,
		Cause:     protocol.AccountUrl("alice", "book", "1").WithTxID([32]byte{2}),
	}

	src = newObservedDB(t)
	b := src.Begin(true)
	ledger := &protocol.SystemLedger{Url: sysLedger, Index: 204}
	require.NoError(t, b.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, b.Account(sysLedger).Events().Minor().Votes(30).Add(vote))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	b = src.Begin(true)
	data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
	require.NoError(t, err)
	idx, err := b.Account(sysLedger).RootChain().Index().Get()
	require.NoError(t, err)
	require.NoError(t, idx.AddEntry(data, false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	b = src.Begin(false)
	root, err = b.GetBptRootHash()
	require.NoError(t, err)
	b.Discard()
	return src, root, ledger.Index, part, sysLedger, vote, partitionID
}

// TestTheEventsBlockListsAreDerivedNotTaken (review F1). The ledger's leaf
// hashes the events BPT root, which holds the events and not the block lists
// the executor finds them by (events.go getBlocksAs; msg_transaction.go
// getBlocksWithEvents). A peer that serves the vote and leaves its block off
// the list -- or adds a block that holds nothing -- passes the leaf check
// either way, so the list the joined node keeps must be derived from the sets
// it verified, never taken from the answer. Taken, the joined node never
// releases the vote at the anchor its peers release it at.
func TestTheEventsBlockListsAreDerivedNotTaken(t *testing.T) {
	src, _, _, part, sysLedger, _, partitionID := ledgerWithOneVote(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, WithReceipt: true, Partition: part}
	ctx := context.Background()

	for _, c := range []struct {
		name string
		peer Source
	}{
		{"control: honest", honest},
		{"the vote is served, its block is left off the list",
			rewriteEvents{Source: honest, fn: func(ev *api.LedgerEvents) { ev.MinorBlocks = nil }}},
		{"a block that holds nothing is added to the list",
			rewriteEvents{Source: honest, fn: func(ev *api.LedgerEvents) { ev.MinorBlocks = []uint64{30, 77} }}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dst := newObservedDB(t)
			batch := dst.Begin(true)
			defer batch.Discard()
			require.NoError(t, Account(ctx, c.peer, batch, sysLedger, opts))
			blocks, err := batch.Account(sysLedger).Events().Minor().Blocks().Get()
			require.NoError(t, err)
			require.Equal(t, []uint64{30}, blocks,
				"the block list is not the one the verified events imply; the executor finds votes by it")
		})
	}
}

// TestStaleMinorVotesAreCleared (review F5): the restart case for the minor
// side, which the suite did not cover -- skipping the clear passed green. A
// restarted node holds a vote the peer has already released; the peer serves
// no events; the pull must clear the node's vote or the ledger is refused by
// every peer for ever -- the #4399 symptom, on the minor side.
func TestStaleMinorVotesAreCleared(t *testing.T) {
	partitionID := "StaleVotes"
	part := protocol.PartitionUrl(partitionID)
	sysLedger := part.JoinPath(protocol.Ledger)
	vote := &protocol.AuthoritySignature{
		Origin:    protocol.AccountUrl("alice", "book", "1"),
		Authority: protocol.AccountUrl("alice", "book"),
		Vote:      protocol.VoteTypeAccept,
		TxID:      protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{1}),
		Cause:     protocol.AccountUrl("alice", "book", "1").WithTxID([32]byte{2}),
	}

	src := newObservedDB(t)
	b := src.Begin(true)
	ledger := &protocol.SystemLedger{Url: sysLedger, Index: 204}
	require.NoError(t, b.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	b = src.Begin(true)
	data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
	require.NoError(t, err)
	idx, err := b.Account(sysLedger).RootChain().Index().Get()
	require.NoError(t, err)
	require.NoError(t, idx.AddEntry(data, false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, WithReceipt: true, Partition: part}

	dst := newObservedDB(t)
	// The stale vote is committed, as a restarted node's store holds it.
	sb := dst.Begin(true)
	require.NoError(t, sb.Account(sysLedger).Main().Put(&protocol.SystemLedger{Url: sysLedger, Index: 190}))
	require.NoError(t, sb.Account(sysLedger).Events().Minor().Votes(6).Add(vote))
	require.NoError(t, sb.UpdateBPT())
	require.NoError(t, sb.Commit())
	sb = dst.Begin(false)
	staleRoot, err := sb.Account(sysLedger).Events().BPT().GetRootHash()
	require.NoError(t, err)
	sb.Discard()
	require.NotEqual(t, [32]byte{}, staleRoot, "precondition: the stale vote is in the joiner's events BPT")

	batch := dst.Begin(true)
	defer batch.Discard()
	require.NoError(t, Account(context.Background(), honest, batch, sysLedger, opts),
		"a stale held vote on the joiner was not cleared by the peer's empty answer")
	got, err := batch.Account(sysLedger).Events().Minor().Votes(6).Get()
	require.NoError(t, err)
	require.Empty(t, got, "the stale vote survived the pull")

	// And the block list that indexed it (review R3): it is outside the
	// events BPT, so no leaf check sees a stale entry left in it.
	blocks, err := batch.Account(sysLedger).Events().Minor().Blocks().Get()
	require.NoError(t, err)
	require.Empty(t, blocks, "the stale vote's block survived in the minor block list")
}

// TestServedEventsAreWrittenOnce (review R2). The events BPT is keyed by
// entry, so an entry served twice leaves its root -- and the ledger's leaf
// check -- unchanged, while the event set itself would hold the entry twice
// and the executor would release a vote or expire a transaction twice. Each
// set is written with each entry once, and a block served twice is one block.
func TestServedEventsAreWrittenOnce(t *testing.T) {
	src, _, _, part, sysLedger, _, partitionID := ledgerWithOneVote(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, WithReceipt: true, Partition: part}
	ctx := context.Background()

	t.Run("the vote is served twice in its block", func(t *testing.T) {
		liar := rewriteEvents{Source: honest, fn: func(ev *api.LedgerEvents) {
			for _, v := range ev.MinorVotes {
				v.Votes = append(v.Votes, v.Votes[0].Copy())
			}
		}}
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		defer batch.Discard()
		err := Account(ctx, liar, batch, sysLedger, opts)
		require.NoError(t, err, "an honest ledger with duplicated entries should be kept, deduplicated")
		votes, err := batch.Account(sysLedger).Events().Minor().Votes(30).Get()
		require.NoError(t, err)
		require.Len(t, votes, 1, "the joined node holds the vote twice; the executor releases it twice")
		blocks, err := batch.Account(sysLedger).Events().Minor().Blocks().Get()
		require.NoError(t, err)
		require.Equal(t, []uint64{30}, blocks)
	})

	t.Run("the block is served twice", func(t *testing.T) {
		liar := rewriteEvents{Source: honest, fn: func(ev *api.LedgerEvents) {
			ev.MinorVotes = append(ev.MinorVotes, ev.MinorVotes[0])
		}}
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		defer batch.Discard()
		require.NoError(t, Account(ctx, liar, batch, sysLedger, opts))
		votes, err := batch.Account(sysLedger).Events().Minor().Votes(30).Get()
		require.NoError(t, err)
		require.Len(t, votes, 1)
		blocks, err := batch.Account(sysLedger).Events().Minor().Blocks().Get()
		require.NoError(t, err)
		require.Equal(t, []uint64{30}, blocks, "a block listed twice")
	})

	t.Run("a pending expiry is served twice", func(t *testing.T) {
		// A second peer fixture: one major pending expiry.
		src2 := newObservedDB(t)
		b := src2.Begin(true)
		ledger := &protocol.SystemLedger{Url: sysLedger, Index: 204}
		require.NoError(t, b.Account(sysLedger).Main().Put(ledger))
		pend := protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{5})
		require.NoError(t, b.Account(sysLedger).Events().Major().Pending(7).Add(pend))
		require.NoError(t, b.Account(sysLedger).Events().Backlog().Expired().Add(pend))
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
		b = src2.Begin(true)
		data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
		require.NoError(t, err)
		idx, err := b.Account(sysLedger).RootChain().Index().Get()
		require.NoError(t, err)
		require.NoError(t, idx.AddEntry(data, false))
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
		honest2 := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src2, Partition: partitionID})}
		opts2 := Options{Mode: ModeStateOnly, WithReceipt: true, Partition: part}
		liar := rewriteEvents{Source: honest2, fn: func(ev *api.LedgerEvents) {
			for _, p := range ev.MajorPending {
				p.Pending = append(p.Pending, p.Pending[0])
			}
			ev.Expired = append(ev.Expired, ev.Expired[0])
		}}
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		defer batch.Discard()
		err = Account(ctx, liar, batch, sysLedger, opts2)
		require.NoError(t, err, "an honest ledger with duplicated entries should be kept, deduplicated")
		pending, err := batch.Account(sysLedger).Events().Major().Pending(7).Get()
		require.NoError(t, err)
		require.Len(t, pending, 1, "the joined node holds the expiry twice")
		expired, err := batch.Account(sysLedger).Events().Backlog().Expired().Get()
		require.NoError(t, err)
		require.Len(t, expired, 1, "the joined node holds the backlog entry twice")
	})

}
