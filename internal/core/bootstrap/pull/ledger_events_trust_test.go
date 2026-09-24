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
	src, root, block, part, sysLedger, _, partitionID := ledgerWithOneVote(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}
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

// TestALoneScheduledEventIsBoundToItsBlock is review F2, a known limit and
// skipped until it is closed. The events BPT hashes values and not keys, and
// a one-sided branch passes its child's hash up, so a tree with one entry has
// root == that entry's hash wherever its key sits: a peer that moves the one
// held vote from block 30 to 31 passes the leaf check. With two or more
// entries the positions bind. Closing it needs the events leaf to hash its
// key, a consensus hash change (DIFFERENCES.md E11).
func TestALoneScheduledEventIsBoundToItsBlock(t *testing.T) {
	t.Skip("known limit (DIFFERENCES.md E11, #4399 review F2): a one-entry events BPT does not bind the entry's block; needs a hash change")

	src, root, block, part, sysLedger, _, partitionID := ledgerWithOneVote(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}
	liar := rewriteEvents{Source: honest, fn: func(ev *api.LedgerEvents) {
		ev.MinorBlocks = []uint64{31}
		for _, v := range ev.MinorVotes {
			v.Block = 31
		}
	}}
	dst := newObservedDB(t)
	batch := dst.Begin(true)
	defer batch.Discard()
	require.Error(t, Account(context.Background(), liar, batch, sysLedger, opts),
		"the one held vote moved from block 30 to 31 and the leaf check passed")
}
