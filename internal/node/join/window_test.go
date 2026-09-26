// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// writeBlockPool writes a pool of n anchors, one a block from block 1, each
// an entry of its anchor-sequence chain with the message behind it and the
// index entry that says which block wrote it, as a partition's own blocks
// write them.
func writeBlockPool(t *testing.T, db *database.Database, pool *url.URL, n int) {
	t.Helper()
	b := db.Begin(true)
	require.NoError(t, b.Account(pool).Main().Put(&protocol.DataAccount{Url: pool}))
	for i := 0; i < n; i++ {
		body := new(protocol.DirectoryAnchor)
		body.Source = protocol.DnUrl()
		body.MinorBlockIndex = uint64(i + 1)
		body.RootChainIndex = uint64(i)
		txn := new(protocol.Transaction)
		txn.Header.Principal = pool
		txn.Body = body
		msg := &messaging.TransactionMessage{Transaction: txn}
		h := msg.Hash()
		require.NoError(t, b.Message(h).Main().Put(msg))
		c := b.Account(pool).AnchorSequenceChain()
		require.NoError(t, c.Inner().AddEntry(h[:], false))
		index, err := c.Index().Get()
		require.NoError(t, err)
		e, err := (&protocol.IndexEntry{Source: uint64(i), BlockIndex: uint64(i + 1)}).MarshalBinary()
		require.NoError(t, err)
		require.NoError(t, index.AddEntry(e, false))
		if (i+1)%1000 == 0 {
			require.NoError(t, b.Commit())
			b = db.Begin(true)
		}
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// countingSource counts the chain entries and messages a peer serves.
type countingSource struct {
	pull.Source
	entries, messages atomic.Int64
}

func (c *countingSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	r, err := c.Source.QueryChainEntries(ctx, u, q)
	if r != nil {
		c.entries.Add(int64(len(r.Records)))
	}
	return r, err
}

func (c *countingSource) QueryMessage(ctx context.Context, id *url.TxID, q *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	c.messages.Add(1)
	return c.Source.QueryMessage(ctx, id, q)
}

// TestJoin_ThePoolIsTakenByItsWindowNotItsHistory — #4452. On the 24h
// network no follower finished its first walk past ~68,900 blocks: the join
// took the anchor pool whole, every entry since genesis with the message
// behind it, so what a join cost grew with the network's age. The pool is
// now taken by its head, its open mark set and the window the executor reads
// (pull.AnchorPoolWindow), and backfilled after the match: what the join
// fetches before its handoff is the same for a pool of 5,000 blocks as for
// one of 20,000. The rest comes with the backfill, and the chain is then the
// peer's entry by entry.
func TestJoin_ThePoolIsTakenByItsWindowNotItsHistory(t *testing.T) {
	here := protocol.DnUrl()
	pool := here.JoinPath(protocol.AnchorPool)

	type cost struct{ entries, messages int64 }
	join := func(n int) cost {
		t.Helper()
		peer := openDisk(t, t.TempDir())
		t.Cleanup(func() { _ = peer.Close() })
		writeBlockPool(t, peer, pool, n)
		node := openDisk(t, t.TempDir())
		t.Cleanup(func() { _ = node.Close() })

		src := &countingSource{Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}}
		s := diskState(node, here, servedAt{Source: src, db: peer, block: uint64(n)})
		p := newSyncing()
		began := time.Now()
		require.Equal(t, taken, s.pullOne(context.Background(), p, pool), "the pool was not taken: %v", p.retry)
		took := time.Since(began)
		c := cost{src.entries.Load(), src.messages.Load()}
		t.Logf("a pool of %d blocks: %d chain entries and %d messages fetched before the handoff, in %v", n, c.entries, c.messages, took)

		// The node holds the peer's head and nothing below the window.
		requireSameHash(t, peer, node, pool)
		b := node.Begin(false)
		_, err := b.Account(pool).AnchorSequenceChain().Inner().Entry(0)
		b.Discard()
		require.Error(t, err, "the pool's first entry was taken before the match")
		require.Contains(t, s.headOnly, accountKey(pool), "the pool is not marked to be backfilled")

		// After the match, the backfill brings the rest.
		require.True(t, s.backfillOne(context.Background(), pool), "the pool was not backfilled")
		requireSameChain(t, peer, node, pool, "anchor-sequence")
		return c
	}

	// What the window costs, whatever the history: the chain and its index
	// chain from a mark point at most a set below the window (the messages
	// come with the entries), the index chain searched for the window a page
	// at a time from its newest entry, and the state at each window's start.
	const markFreq, page = 256, 256
	bound := int64(2*(pull.AnchorPoolWindow+markFreq) + pull.AnchorPoolWindow + page + 2)
	for _, n := range []int{5_000, 20_000} {
		c := join(n)
		require.LessOrEqual(t, c.entries, bound, "a join against a pool of %d blocks fetched more than its window before the handoff", n)
	}
}

// requireSameHash holds the node's account to the peer's hash.
func requireSameHash(t *testing.T, peer, node *database.Database, u *url.URL) {
	t.Helper()
	pb := peer.Begin(false)
	defer pb.Discard()
	nb := node.Begin(false)
	defer nb.Discard()
	want, err := pb.Account(u).Hash()
	require.NoError(t, err)
	got, err := nb.Account(u).Hash()
	require.NoError(t, err)
	require.Equal(t, want, got, "the node's account does not hash as the peer's")
}
