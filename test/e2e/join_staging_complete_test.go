// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAJoinHoldsWhatItsPeersHeldBeforeItListened is #4442 step 6 (executor
// spec, "Sync", "The algorithm", step 6; #4412 decision B). BVN0 sends to BVN1
// every block, and every Directory anchor reaches BVN1 ten blocks late, so
// BVN1's validators always hold synthetics received and waiting on the anchor
// that proves them. A BVN1 node restarts and the traffic stops: what its peers
// held when it started listening, it never collects. The state it is proven
// at says, per stream, what was Received; the entries between Delivered and
// Received are what its peers will execute when the anchors land.
//
// The follower hands off only once it holds every entry from Delivered to
// Received: here, once its peers have executed what they held. It executes
// its blocks in agreement with them. Before, it handed off with the gap and
// executed the blocks that carried the peers' held synthetics without them.
func TestAJoinHoldsWhatItsPeersHeldBeforeItListened(t *testing.T) {
	const anchorLag = 10
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(4096),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	var mu sync.Mutex
	type late struct {
		at  int
		env *messaging.Envelope
	}
	blocks := map[int]int{}
	pending := map[int][]late{}
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ coreexec.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		mu.Lock()
		defer mu.Unlock()
		blocks[node]++
		now := blocks[node]
		var kept, due []*messaging.Envelope
		rest := pending[node][:0]
		for _, l := range pending[node] {
			if l.at <= now {
				due = append(due, l.env)
			} else {
				rest = append(rest, l)
			}
		}
		pending[node] = rest
		for _, env := range envelopes {
			if isDirectoryAnchor(env) {
				pending[node] = append(pending[node], late{now + anchorLag, env})
				continue
			}
			kept = append(kept, env)
		}
		return append(due, kept...), true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e6))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	var sts []*TransactionStatus
	step := func(n int, traffic bool) {
		for i := 0; i < n; i++ {
			if traffic {
				for j := 0; j < 2; j++ {
					ts++
					sts = append(sts, sim.BuildAndSubmitTxnSuccessfully(
						build.Transaction().For(alice, "tokens").
							SendTokens(1, 0).To(bob, "tokens").
							SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)))
				}
			}
			sim.Step()
		}
	}
	step(40, true)
	// The traffic stops. What BVN0 sent reaches BVN1 within a few blocks,
	// and waits there on the anchors that prove it.
	step(6, false)

	p := sim.S.Partition("BVN1")
	part := PartitionUrl("BVN1")
	peersHeld := heldNumbers(p.NodeStaging(0))
	held := 0
	for _, h := range peersHeld {
		held += len(h)
	}
	require.NotZero(t, held, "precondition: BVN1's validators hold entries at the restart")

	// The node restarts, twice, as a process does: it collected nothing of
	// what its peers held.
	p.RestartNode(1)
	p.RestartNode(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 200
	st := &provenState{}
	st.steppingState = steppingState{cancel: cancel, step: func(round int) {
		step(1, false)
		if round >= maxRounds {
			cancel()
		}
	}}
	st.State = p.NodeJoinState(1)
	settler, ok := p.NodeExecutor(1).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN1",
		Buffer:    p.NodeJoin(1),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(1), Database: p.NodeDatabase(1), Partition: part},
		State:     st,
		Peers:     &join.APIPeers{Partition: "BVN1", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not complete within %d pull rounds", maxRounds)
	require.NotEmpty(t, st.handoffs, "the join never handed off")
	require.Zero(t, st.diverged, "the follower executed a block other than its peers' %d times after handing off (handoffs at %v)",
		st.diverged, handoffBlocks(st))

	// The anchors land; the peers execute what they held. So must the
	// follower, block for block.
	state := p.NodeJoinState(1)
	for i := 0; i < 3*anchorLag; i++ {
		step(1, false)
		n, diverged, err := state.Diverged(context.Background())
		require.NoError(t, err)
		require.False(t, diverged, "the follower's block %d differs from its peers' (%v)", n, heldSummary(state))
	}
	var anchors [][]byte
	for i := 0; i < p.NodeCount(); i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "BVN1 node %d's root chain differs from node 0's after the join", i)
	}
}

func heldSummary(state *join.PulledState) any {
	n, diffs := state.Mismatches()
	return struct {
		Block uint64
		Diffs []join.LeafDiff
	}{n, diffs}
}

func handoffBlocks(st *provenState) []uint64 {
	var out []uint64
	for _, h := range st.handoffs {
		out = append(out, h.block)
	}
	return out
}
