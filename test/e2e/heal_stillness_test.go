// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
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

// #4415. The pull pair rotates with every activation (healing.md, "Who asks,
// and when"), and a synthetic stream is asked about only once its Delivered
// has sat still for probeAfter activations. Stillness used to be counted only
// on the activations a node was in the pair, so once the pair rotated, a
// node's count grew at the rate it was selected: about two in N activations.
// A shared hole then waited for some node to have been selected probeAfter
// times while the stream sat still -- about N/2 times the stillness window.
// Stillness is now observed on every node at every activation, so the first
// node selected after the window asks, whoever it is.
//
// This drives the production wiring end to end -- the conductor's block hook,
// the real seed read from each node's store, staging, the private sequencer
// service -- on a four-validator BVN and on a twelve-validator Directory, and
// requires every shared hole to heal within the stillness window plus four
// activations. The window is probeAfter (8) activations of healCadence (4)
// blocks: 32 steps. Measured with the fix, every hole healed in 32-33 steps on
// both; with stillness counted only when selected, 44-60 steps on the BVN and
// 77-128 on the Directory.
func TestASharedHoleHealsWithinOneStillnessWindow(t *testing.T) {
	const bound = 8*4 + 16
	cases := []struct {
		name        string
		bvns, nodes int
		dest        string
		validators  int
	}{
		{"BVN of 4", 2, 4, "BVN1", 4},
		{"Directory of 12", 3, 4, Directory, 12},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			steps := measureSharedHoles(t, c.bvns, c.nodes, c.dest, c.validators, 6)
			for h, n := range steps {
				require.LessOrEqualf(t, n, bound,
					"hole %d took %d steps to heal on %s; the stillness window is 32 and the bound %d: stillness is not counted on every activation",
					h, n, c.dest, bound)
			}
		})
	}
}

// measureSharedHoles opens a hole every node of dest shares on the BVN0 ->
// dest synthetic stream, holes times in turn, while dest is kept busy with
// local traffic so every block anchors, and returns how many steps each hole
// took to heal on every node. Every node drops the first copy of the hole's
// number it is handed; a heal's copy goes through.
func measureSharedHoles(t *testing.T, bvns, nodes int, dest string, validators, holes int) []int {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	carol := url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	carolKey := acctesting.GenerateKey(carol)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), bvns, nodes),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, dest)
	sim.SetRoute(carol, dest)

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

	var mu sync.Mutex
	var hole, maxSeen uint64
	dropped := map[int]bool{}
	sim.S.SetNodeBlockHook(dest, func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
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

	p := sim.S.Partition(dest)
	delivered := func(i int) uint64 {
		var d uint64
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			var ledger *SyntheticLedger
			require.NoError(t, batch.Account(PartitionUrl(dest).JoinPath(Synthetic)).Main().GetAs(&ledger))
			d = ledger.Partition(PartitionUrl("BVN0")).Delivered
		})
		return d
	}

	for i := 0; i < 40; i++ {
		step()
	}

	const window = 400
	var steps []int
	askers := map[int]int{}
	for h := 0; h < holes; h++ {
		before := make([]uint64, validators)
		for i := range before {
			before[i] = p.NodeHeals(i).Requests.Load()
		}
		mu.Lock()
		hole = maxSeen + 2
		dropped = map[int]bool{}
		target := hole
		mu.Unlock()

		n := 0
		for healed := false; !healed; {
			require.Less(t, n, window, "hole %d (number %d) on %s was not healed within %d steps", h, target, dest, window)
			step()
			n++
			healed = true
			for i := 0; i < validators; i++ {
				if delivered(i) <= target {
					healed = false
					break
				}
			}
		}
		var asked []int
		for i := 0; i < validators; i++ {
			if p.NodeHeals(i).Requests.Load() > before[i] {
				asked = append(asked, i)
				askers[i]++
			}
		}
		steps = append(steps, n)
		t.Logf("%s: hole %d at %d healed in %d steps; nodes that asked: %v", dest, h, target, n, asked)
	}
	t.Logf("%s (%d validators): steps to heal per hole %v; askers %v", dest, validators, steps, askers)
	return steps
}
