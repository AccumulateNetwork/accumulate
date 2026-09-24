// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// #4415, run 20260924T074702Z. The Directory's heal requests came from the
// same two validators at every activation of the run: acc-bvn2-val4 logged a
// request at 358 distinct activation seconds, acc-bvn3-val1 at every
// activation until its 07:59 restart, and the other ten 0-19 times each.
// When acc-bvn3-val1's Directory stopped executing at block 661 the
// Directory was left with one requester for the rest of the run.
//
// healing.md ("Who asks, and when") and selectedToPull say the pair is drawn
// per activation from the previous block's hash, so the load and the single
// point of failure rotate. The hash it reads, previousBlockSeed, is the
// STORED ledger anchor's RootChainAnchor, and the executor stores that anchor
// without it (block_end.go, buildDirectoryAnchor / buildPartitionAnchor: "Do
// not populate the root chain anchor ... until the next block starts"; only
// the copy ConstructLastAnchor sends gets it). On every block that anchors,
// the seed is 32 zero bytes and pullSenders names the same two positions.
// On a busy partition every block anchors, so the pair never moves.
//
// This keeps BVN1 busy with local traffic so every block anchors, opens a
// hole every node shares on BVN0 -> BVN1 (each node drops the first copy of
// one number; a heal's copy goes through), waits for it to heal, and repeats.
// Over eight holes, drawn per activation from a moving seed, more than two of
// the four validators must have asked. It fails with exactly two.
func TestHealRequestersRotateOnABusyPartition(t *testing.T) {
	const nodes = 4
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	carol := url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	carolKey := acctesting.GenerateKey(carol)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, nodes),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	sim.SetRoute(carol, "BVN1")

	seqOf := func(m messaging.Message) (uint64, bool) {
		syn, ok := m.(*messaging.SyntheticMessage)
		if !ok {
			return 0, false
		}
		seq, ok := syn.Message.(*messaging.SequencedMessage)
		if !ok || seq.Source == nil || !seq.Source.Equal(PartitionUrl("BVN0")) {
			return 0, false
		}
		return seq.Number, true
	}

	// Every node drops the first copy of `hole` it is handed, and nothing
	// else: the hole is shared, and whichever node asks, its answer fills it
	// on every node.
	var mu sync.Mutex
	var hole, maxSeen uint64
	dropped := map[int]bool{}
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		mu.Lock()
		defer mu.Unlock()
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			var rest []messaging.Message
			touched := false
			for _, m := range env.Messages {
				if n, ok := seqOf(m); ok {
					if n > maxSeen {
						maxSeen = n
					}
					if hole != 0 && n == hole && !dropped[node] {
						dropped[node] = true
						touched = true
						continue
					}
				}
				rest = append(rest, m)
			}
			switch {
			case !touched:
				kept = append(kept, env)
			case len(rest) > 0:
				e := env.Copy()
				e.Messages = rest
				kept = append(kept, e)
			}
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(bob), bob.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(carol), carol, carolKey[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	ts := uint64(0)
	// One cross-partition transfer (the stream the holes are on) and one
	// BVN1-local transfer (so every BVN1 block has something to anchor, as
	// every Directory block did at 100 tps on the soak).
	step := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(bob, "tokens").
				SendTokens(1, 0).To(carol, "tokens").
				SignWith(bob, "book", "1").Version(1).Timestamp(ts).PrivateKey(bobKey))
		sim.Step()
	}

	p := sim.S.Partition("BVN1")
	delivered := func(i int) uint64 {
		var d uint64
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			var ledger *SyntheticLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN1").JoinPath(Synthetic)).Main().GetAs(&ledger))
			d = ledger.Partition(PartitionUrl("BVN0")).Delivered
		})
		return d
	}
	anchored := func() bool {
		var a bool
		View(t, p.NodeDatabase(0), func(batch *database.Batch) {
			var l *SystemLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN1").JoinPath(Ledger)).Main().GetAs(&l))
			a = l.Anchor != nil
		})
		return a
	}

	for i := 0; i < 40; i++ {
		step()
	}

	const holes = 8
	const window = 200
	askers := map[int]int{}
	steps, unanchored := 0, 0
	for h := 0; h < holes; h++ {
		before := make([]uint64, nodes)
		for i := range before {
			before[i] = p.NodeHeals(i).Requests.Load()
		}
		mu.Lock()
		hole = maxSeen + 2
		dropped = map[int]bool{}
		target := hole
		mu.Unlock()

		healed := false
		for b := 0; b < window; b++ {
			step()
			steps++
			if !anchored() {
				unanchored++
			}
			all := true
			for i := 0; i < nodes; i++ {
				if delivered(i) <= target {
					all = false
				}
			}
			if all {
				healed = true
				break
			}
		}
		require.True(t, healed, "hole %d (number %d) was not healed within %d steps", h, target, window)
		var asked []int
		for i := 0; i < nodes; i++ {
			if p.NodeHeals(i).Requests.Load() > before[i] {
				asked = append(asked, i)
				askers[i]++
			}
		}
		t.Logf("hole %d at %d healed; nodes that asked: %v", h, target, asked)
	}

	var who []int
	for i := range askers {
		who = append(who, i)
	}
	sort.Ints(who)
	t.Logf("over %d holes and %d steps (%d with an unanchored ledger), the nodes that ever asked: %v (%v)",
		holes, steps, unanchored, who, askers)
	require.Greater(t, len(who), sendersPerActivation,
		"the same %d validators asked for every hole: the pull pair does not rotate on a busy partition", len(who))
}

// sendersPerActivation mirrors crosschain.sendersPerActivation.
const sendersPerActivation = 2
