// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestABVNNodeBehindAcrossASyntheticMarkPointJoins — #4434. A BVN validator
// restarts a few blocks behind while traffic keeps flowing, and its own
// synthetic chain to the Directory crosses a mark point (every 256 entries)
// in the blocks it does not execute. The production join -- join.Run over the
// node's own buffer, executor and join state, reading named peers through
// the real client -- must pull, match and hand off.
//
// On issue-4205-lead @ 4cd5e684e every handoff failed in the seed, as
// acc-bvn1-val1's BVN1 did 25 times in run 20260924T111811Z:
//
//	produce block: begin block: seed synthetic cache: rebuild cache for block
//	N: load synthetic chain state for acc://dn.acme before F: mark point 255
//	of Account.acc://bvn-BVN0.acme/synthetic.SyntheticChain.directory is
//	missing; cannot compute the state at F-1
//
// The join pulled <partition>/synthetic state-only: the peer's head and its
// open mark set, and not the mark point that closed the set before it. The
// seed read the chain's state before the block's first entry before it
// looked for the entries, so the skip rule for blocks the node did not
// execute never ran.
func TestABVNNodeBehindAcrossASyntheticMarkPointJoins(t *testing.T) {
	const joiner = 1

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")
	// bob is on the Directory, so every send puts a deposit on BVN0's
	// synthetic chain to the Directory.
	sim.SetRoute(bob, "Directory")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e6))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	submit := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	bvn := PartitionUrl("BVN0")
	part := sim.S.Partition("BVN0")
	toDirectory := func(db *database.Database) int64 {
		var n int64
		View(t, db, func(batch *database.Batch) {
			head, err := batch.Account(bvn.JoinPath(Synthetic)).SyntheticChain(Directory).Head().Get()
			require.NoError(t, err)
			n = head.Count
		})
		return n
	}

	// Load the chain to just short of its first mark point, on every node.
	const markFreq = 256
	for toDirectory(part.NodeDatabase(joiner)) < markFreq-24 {
		for i := 0; i < 20; i++ {
			submit()
		}
		sim.StepN(3)
	}
	sim.StepN(10)
	before := toDirectory(part.NodeDatabase(joiner))
	require.Less(t, before, int64(markFreq), "precondition: the node's own chain has not reached the mark point")
	r := partitionBlock(t, part.NodeDatabase(joiner), bvn)

	// The restart: the node collects and executes nothing, and the chain
	// crosses the mark point in the blocks it does not execute.
	part.RestartNode(joiner)
	require.True(t, part.Joining(joiner))
	for toDirectory(part.NodeDatabase(0)) < markFreq+24 {
		for i := 0; i < 10; i++ {
			submit()
		}
		sim.StepN(3)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 200
	stepping := &steppingState{State: part.NodeJoinState(joiner), step: func(round int) {
		// Traffic continues while the node pulls, so the blocks it hands off
		// at carry synthetics still in flight.
		for i := 0; i < 2; i++ {
			submit()
		}
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}
	settler, ok := part.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	buffer := &handoffCounting{Buffer: part.NodeJoin(joiner)}
	outcome, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buffer,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: part.NodeStaging(joiner), Database: part.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	for i, e := range buffer.failed {
		t.Logf("handoff failure %d: %v", i+1, e)
	}
	require.Empty(t, buffer.failed, "a handoff failed to produce a block")
	require.NoError(t, err, "the node did not join within %d pull rounds", maxRounds)
	require.Equal(t, join.Joined, outcome)
	require.False(t, part.Joining(joiner))

	q := partitionBlock(t, part.NodeDatabase(joiner), bvn)
	t.Logf("the node stopped at R=%d with %d synthetic entries to the Directory, and is now at %d with %d", r, before, q, toDirectory(part.NodeDatabase(joiner)))
	require.Greater(t, toDirectory(part.NodeDatabase(joiner)), int64(markFreq),
		"precondition: the chain crossed the mark point in blocks the node did not execute")

	// The node executes on from the handoff.
	for i := 0; i < 3; i++ {
		submit()
	}
	sim.StepN(10)
	require.Greater(t, partitionBlock(t, part.NodeDatabase(joiner), bvn), q, "the joined node executes after the handoff")
	require.Empty(t, syntheticEntriesWithNoMessage(t, part.NodeDatabase(joiner), "BVN0"),
		"the joined node holds synthetic ledger entries with no message behind them")
}

// syntheticEntriesWithNoMessage is entriesWithNoMessage for the partition's
// synthetic ledger, which the join takes whole like a spine account (#4434):
// for each of its chains whose entries are the hashes of messages, the
// positions held with no message behind them, or not held at all.
func syntheticEntriesWithNoMessage(t *testing.T, db *database.Database, partition string) map[string][]int64 {
	t.Helper()
	out := map[string][]int64{}
	u := PartitionUrl(partition).JoinPath(Synthetic)
	View(t, db, func(batch *database.Batch) {
		chains, err := batch.Account(u).Chains().Get()
		require.NoError(t, err)
		for _, meta := range chains {
			if meta.Type != ChainTypeTransaction || strings.HasPrefix(meta.Name, "synthetic-sequence(") {
				continue
			}
			c, err := batch.Account(u).ChainByName(meta.Name)
			require.NoError(t, err)
			head, err := c.Head().Get()
			require.NoError(t, err)
			for i := int64(0); i < head.Count; i++ {
				h, err := c.Inner().Entry(i)
				if err != nil {
					out[meta.Name] = append(out[meta.Name], i)
					continue
				}
				if _, err := batch.Message2(h).Main().Get(); err != nil {
					out[meta.Name] = append(out[meta.Name], i)
				}
			}
		}
	})
	for k, v := range out {
		t.Logf("%v#%s: %d entries with no message behind them", u, k, len(v))
	}
	return out
}
