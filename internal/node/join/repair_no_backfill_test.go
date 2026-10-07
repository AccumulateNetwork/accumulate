// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// writeTokenHistory writes a token account whose main chain holds n
// transactions, each with the message behind it, as a peer that executed them
// holds it.
func writeTokenHistory(t *testing.T, db *database.Database, u *url.URL, n int) {
	t.Helper()
	b := db.Begin(true)
	require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(int64(n))}))
	for i := 0; i < n; i++ {
		txn := new(protocol.Transaction)
		txn.Header.Principal = u
		txn.Body = &protocol.SendTokens{To: []*protocol.TokenRecipient{{Url: protocol.AccountUrl("carol", "tokens"), Amount: *big.NewInt(int64(i + 1))}}}
		msg := &messaging.TransactionMessage{Transaction: txn}
		h := msg.Hash()
		require.NoError(t, b.Message(h).Main().Put(msg))
		require.NoError(t, b.Account(u).MainChain().Inner().AddEntry(h[:], false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// TestARepairBackfillsNothing — #4519; executor spec, "Sync", "Two
// mismatches", 2 (Paul, 2026-10-04: "Syncing should NEVER do backfills. The
// point of Syncing is to catch up, follow the network, and do backfills in the
// background"). A repair takes the accounts a block's record names as the join
// takes any account: state, chain heads and open mark sets. It fetches no
// entry below the open set and no message, and leaves the account to the
// background backfill. On run
// 20261004T151138Z a repair that took every named account's history held three
// followers BOOTING for up to 13 minutes at 10 tps.
func TestARepairBackfillsNothing(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	bob := protocol.AccountUrl("bob", "tokens")

	for _, path := range []string{"pullOne", "records"} {
		t.Run(path, func(t *testing.T) {
			peer := openDisk(t, t.TempDir())
			t.Cleanup(func() { _ = peer.Close() })
			writeTokenHistory(t, peer, bob, 600)
			node := openDisk(t, t.TempDir())
			t.Cleanup(func() { _ = node.Close() })

			src := &countingSource{Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}}
			served := servedAt{Source: src, db: peer, block: 7}
			s := diskState(node, here, served)
			p := newSyncing()
			p.repair = true
			ctx := context.Background()

			switch path {
			case "pullOne":
				require.Equal(t, taken, s.pullOne(ctx, p, bob, false), "bob was not taken: %v", p.retry)
			case "records":
				st, err := served.QueryAccountStates(ctx, here, &api.AccountStatesQuery{Accounts: []*url.URL{bob}})
				require.NoError(t, err)
				require.Len(t, st.States, 1)
				got := s.takeStates(ctx, p, served, st.States, []uint64{7})
				require.True(t, got[accountKey(bob)], "bob was not taken")
			}

			// What every join takes of a chain: its head and its open mark
			// set, the entries above its last mark point, which it appends
			// to. Nothing below.
			const markFreq = 256
			require.LessOrEqual(t, src.entries.Load(), int64(600%markFreq), "the repair fetched chain entries below the open mark set")
			require.Zero(t, src.messages.Load(), "the repair fetched messages")
			requireSameHash(t, peer, node, bob)
			require.Contains(t, s.headOnly, accountKey(bob), "bob is not left to the background backfill")

			// The background backfill, past the match, brings the history.
			require.True(t, s.backfillOne(ctx, bob), "bob was not backfilled")
			requireSameChain(t, peer, node, bob, "main")
		})
	}
}

// TestARepairedChainTheNodeGrewWronglyIsReplacedByTheBackfill — #4519;
// executor spec, "Sync", §3: "In a repair nothing is taken whole: the node's
// longer chain is its own wrong growth, its head is replaced by the peer's,
// and the background backfill replaces the entries below it". The node holds
// the peer's first 300 (or all 600) entries of bob's main chain and 3 of its
// own after them; the peer holds 600. The repair leaves bob hashing as the peer's, and
// the backfill makes the chain the peer's entry by entry.
func TestARepairedChainTheNodeGrewWronglyIsReplacedByTheBackfill(t *testing.T) {
	// Shorter than the peer's after its own entries (300+3 of 600), and
	// longer (600+3 of 600): a node that executed what the peers did not.
	for _, held := range []int{300, 600} {
		t.Run(fmt.Sprint(held), func(t *testing.T) { repairWrongGrowth(t, held) })
	}
}

func repairWrongGrowth(t *testing.T, held int) {
	here := protocol.PartitionUrl("BVN0")
	bob := protocol.AccountUrl("bob", "tokens")
	peer := openDisk(t, t.TempDir())
	t.Cleanup(func() { _ = peer.Close() })
	writeTokenHistory(t, peer, bob, 600)
	node := openDisk(t, t.TempDir())
	t.Cleanup(func() { _ = node.Close() })
	writeTokenHistory(t, node, bob, held)
	b := node.Begin(true)
	for i := 0; i < 3; i++ {
		txn := new(protocol.Transaction)
		txn.Header.Principal = bob
		txn.Header.Memo = "executed wrongly"
		txn.Body = &protocol.SendTokens{To: []*protocol.TokenRecipient{{Url: protocol.AccountUrl("dave", "tokens"), Amount: *big.NewInt(int64(i + 1))}}}
		msg := &messaging.TransactionMessage{Transaction: txn}
		h := msg.Hash()
		require.NoError(t, b.Message(h).Main().Put(msg))
		require.NoError(t, b.Account(bob).MainChain().Inner().AddEntry(h[:], false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	src := &countingSource{Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}}
	s := diskState(node, here, servedAt{Source: src, db: peer, block: 7})
	p := newSyncing()
	p.repair = true
	ctx := context.Background()
	require.Equal(t, taken, s.pullOne(ctx, p, bob, false), "bob was not taken: %v", p.retry)
	requireSameHash(t, peer, node, bob)
	require.Contains(t, s.headOnly, accountKey(bob), "bob is not left to the background backfill")

	// Below the head (300+3), the first walk finds that the entries the
	// node holds do not reproduce the peer's head and trusts none of them;
	// the next, which the backfill runs a few seconds later, takes every
	// entry below the head (pull.Backfill). Above it (600+3), they are past
	// the head, where the next append writes.
	walks := 1
	for !s.backfillOne(ctx, bob) {
		walks++
		require.LessOrEqual(t, walks, 2, "bob was not backfilled in two walks")
	}
	requireSameChain(t, peer, node, bob, "main")
}
