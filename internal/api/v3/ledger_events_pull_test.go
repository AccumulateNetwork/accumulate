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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAPartitionLedgerWithScheduledEventsCanBePulled is
// TestASyntheticLedgerWithQueuedLocalDeliveriesCanBePulled for the other
// system account whose leaf hashes more than the queries serve: a partition
// ledger commits to the root of its scheduled-events BPT
// (observer_prod.go, hashSecondaryState) -- authority votes held for a minor
// block, pending transactions that expire at a major block, and the expiry
// backlog (sig_common.go, block_end.go, msg_transaction.go). None of it was
// served on the current path or pulled (#4399), so a network holding one
// pending multisig transaction would have refused <partition>/ledger to every
// joining node for ever. The load generator signs singly, which is why no run
// has shown it.
//
// It runs the real querier and the real pull: nothing here is a stub.
func TestAPartitionLedgerWithScheduledEventsCanBePulled(t *testing.T) {
	const partitionID = "LedgerEvents"
	part := protocol.PartitionUrl(partitionID)
	sysLedger := part.JoinPath(protocol.Ledger)

	pending := protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{1})
	vote := &protocol.AuthoritySignature{
		Origin:    protocol.AccountUrl("alice", "book", "1"),
		Authority: protocol.AccountUrl("alice", "book"),
		Vote:      protocol.VoteTypeAccept,
		TxID:      pending,
		Cause:     protocol.AccountUrl("alice", "book", "1").WithTxID([32]byte{2}),
	}
	expired := protocol.AccountUrl("bob", "tokens").WithTxID([32]byte{3})
	stale := protocol.AccountUrl("carol", "tokens").WithTxID([32]byte{4})

	type events struct {
		votes   map[uint64][]*protocol.AuthoritySignature
		pending map[uint64][]*url.TxID
		expired []*url.TxID
	}
	write := func(t *testing.T, e *database.AccountEvents, ev events) {
		for b, v := range ev.votes {
			require.NoError(t, e.Minor().Votes(b).Add(v...))
		}
		for b, p := range ev.pending {
			require.NoError(t, e.Major().Pending(b).Add(p...))
		}
		if len(ev.expired) > 0 {
			require.NoError(t, e.Backlog().Expired().Add(ev.expired...))
		}
	}

	for _, c := range []struct {
		name   string
		peer   events
		joiner events
	}{
		{name: "idle: no events on either side (control)"},
		{name: "the peer holds a vote, a pending expiry and a backlog", peer: events{
			votes:   map[uint64][]*protocol.AuthoritySignature{30: {vote}},
			pending: map[uint64][]*url.TxID{7: {pending}},
			expired: []*url.TxID{expired},
		}},
		{name: "restart: the joiner holds its own stale events", joiner: events{
			pending: map[uint64][]*url.TxID{6: {stale}},
			expired: []*url.TxID{stale},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()

			peerDB := database.OpenInMemory(nil)
			peerDB.SetObserver(database.NewDatabaseObserver())
			batch := peerDB.Begin(true)
			ledger := &protocol.SystemLedger{Url: sysLedger, Index: 204}
			require.NoError(t, batch.Account(sysLedger).Main().Put(ledger))
			write(t, batch.Account(sysLedger).Events(), c.peer)
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())

			batch = peerDB.Begin(true)
			data, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
			require.NoError(t, err)
			root, err := batch.Account(sysLedger).RootChain().Index().Get()
			require.NoError(t, err)
			require.NoError(t, root.AddEntry(data, false))
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())

			batch = peerDB.Begin(false)
			peerRoot, err := batch.GetBptRootHash()
			require.NoError(t, err)
			peerEvents, err := batch.Account(sysLedger).Events().BPT().GetRootHash()
			require.NoError(t, err)
			batch.Discard()

			joinDB := database.OpenInMemory(nil)
			joinDB.SetObserver(database.NewDatabaseObserver())
			jb := joinDB.Begin(true)
			defer jb.Discard()
			write(t, jb.Account(sysLedger).Events(), c.joiner)

			src := api.Querier2{Querier: NewQuerier(QuerierParams{Database: peerDB, Partition: partitionID})}
			p, err := pull.Fetch(ctx, src, jb, sysLedger, pull.Options{Mode: pull.ModeStateOnly, Partition: part}, true)
			require.NoError(t, err)
			require.Equal(t, peerRoot, p.Root(), "the receipt must end at the peer's root")
			require.NoError(t, p.Settle(peerRoot),
				"an honest peer's partition ledger does not verify against the peer's own root")

			// The events themselves, not only their root: the executor
			// processes them at the blocks they name.
			got, err := jb.Account(sysLedger).Events().BPT().GetRootHash()
			require.NoError(t, err)
			require.Equal(t, peerEvents, got)
			for b, want := range c.peer.pending {
				have, err := jb.Account(sysLedger).Events().Major().Pending(b).Get()
				require.NoError(t, err)
				require.Len(t, have, len(want))
				for i := range want {
					require.Equal(t, want[i].String(), have[i].String())
				}
			}
			blocks, err := jb.Account(sysLedger).Events().Major().Blocks().Get()
			require.NoError(t, err)
			require.Len(t, blocks, len(c.peer.pending), "the joiner's major block list is not the peer's")
		})
	}
}
