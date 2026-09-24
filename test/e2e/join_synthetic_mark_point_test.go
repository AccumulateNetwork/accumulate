// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multi "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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
	stepping := &steppingState{cancel: cancel, State: part.NodeJoinState(joiner), step: func(round int) {
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
	// The seed reads the chain's state before a block's first entry, which
	// is computed from the mark point below it: the join must have written
	// every one it can read.
	require.Empty(t, syntheticMarkPointsMissing(t, part.NodeDatabase(joiner), "BVN0"),
		"the joined node lacks a mark point of its synthetic ledger's chains")
	require.Empty(t, syntheticEntriesWithNoMessage(t, part.NodeDatabase(joiner), "BVN0"),
		"the joined node holds synthetic ledger entries with no message behind them")
}

// syntheticMarkPointsMissing is, for each chain of the partition's synthetic
// ledger, the mark points below its head the store does not hold. The seed
// reads the chain's state before a block's first entry, and a state is
// computed from the mark point below it (merkle.Chain.StateAt), so this is
// what "the seed never reads a mark point the pull did not write" rests on.
func syntheticMarkPointsMissing(t *testing.T, db *database.Database, partition string) map[string][]int64 {
	t.Helper()
	out := map[string][]int64{}
	u := PartitionUrl(partition).JoinPath(Synthetic)
	View(t, db, func(batch *database.Batch) {
		chains, err := batch.Account(u).Chains().Get()
		require.NoError(t, err)
		for _, meta := range chains {
			c, err := batch.Account(u).ChainByName(meta.Name)
			require.NoError(t, err)
			head, err := c.Head().Get()
			require.NoError(t, err)
			freq := c.Inner().MarkFreq()
			for m := freq - 1; m < head.Count; m += freq {
				if _, err := c.Inner().States(uint64(m)).Get(); err != nil {
					out[meta.Name] = append(out[meta.Name], m)
				}
			}
		}
	})
	for k, v := range out {
		t.Logf("%v#%s: mark points not held: %v", u, k, v)
	}
	return out
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

// TestTheSeedSkipsAStateOnlySyntheticChainAcrossAMarkPoint — #4434, the
// second line of defence. A store an earlier join wrote holds
// <partition>/synthetic as every join before #4434 took it: the peer's head
// and open mark set, and no mark point below the set. The seed must skip the
// blocks it did not execute on the store's evidence -- their entries not
// held, or held with no message -- before it reads the chain's state before
// their first entry, which it cannot: that state is computed from the mark
// point the pull never wrote.
//
// The store is written here by the pull library, as the earlier join wrote
// it: it is the input, not the join under test.
func TestTheSeedSkipsAStateOnlySyntheticChainAcrossAMarkPoint(t *testing.T) {
	const joiner = 1

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
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
	synth := bvn.JoinPath(Synthetic)
	part := sim.S.Partition("BVN0")
	toDirectory := func(db *database.Database) int64 {
		var n int64
		View(t, db, func(batch *database.Batch) {
			head, err := batch.Account(synth).SyntheticChain(Directory).Head().Get()
			require.NoError(t, err)
			n = head.Count
		})
		return n
	}

	const markFreq = 256
	for toDirectory(part.NodeDatabase(joiner)) < markFreq-24 {
		for i := 0; i < 20; i++ {
			submit()
		}
		sim.StepN(3)
	}
	sim.StepN(10)
	require.Less(t, toDirectory(part.NodeDatabase(joiner)), int64(markFreq), "precondition: the node's own chain has not reached the mark point")

	part.RestartNode(joiner)
	for toDirectory(part.NodeDatabase(0)) < markFreq+24 {
		for i := 0; i < 10; i++ {
			submit()
		}
		sim.StepN(3)
	}
	sim.StepN(10)

	// What an earlier join wrote: the synthetic ledger, state-only.
	peer := api.Querier2{Querier: sim.S.Services().ForPeer(part.NodePeerID(0)).ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr())}
	batch := part.NodeDatabase(joiner).Begin(true)
	require.NoError(t, pull.Account(context.Background(), peer, batch, synth, pull.Options{Mode: pull.ModeStateOnly}))
	require.NoError(t, batch.Commit())
	require.NotEmpty(t, syntheticMarkPointsMissing(t, part.NodeDatabase(joiner), "BVN0"),
		"precondition: the store lacks a mark point of the synthetic chain, as a state-only pull leaves it")

	// A new process opens a block past the blocks the chain crossed the mark
	// point in: its seed reads them.
	peerBlock := partitionBlock(t, part.NodeDatabase(0), bvn)
	x := freshBVN0Executor(t, sim, part.NodeDatabase(joiner), synthcache.New(0))
	_, err := x.Begin(coreexec.BlockParams{Context: context.Background(), Index: peerBlock + 1, Time: time.Now()})
	if err != nil {
		require.NotContains(t, err.Error(), "seed synthetic cache",
			"the seed read the state of a chain whose blocks the store says the node did not execute")
	}
}

// TestTheSyntheticLedgerIsTakenWithItsCompanions — #4434. A synthetic that is
// a signature request (or a signature, or a credit payment) names a
// transaction that is not on the chain, and the seed loads it beside the
// entry (rebuildCacheBlock): a synthetic ledger taken whole must bring it, or
// the seed fails on "load transaction for synthetic message" for a block the
// node did not execute. The pull that takes the account whole is the join's
// (TestABVNNodeBehindAcrossASyntheticMarkPointJoins drives it); this asks the
// pull for what it must bring.
func TestTheSyntheticLedgerIsTakenWithItsCompanions(t *testing.T) {
	const joiner = 1

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "Directory")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	// alice's tokens answer to bob's book too, and bob's book is on the
	// Directory: every send asks it for a signature, a synthetic whose
	// companion is the send (TestAnExecutedBlockMissingACompanionFailsTheSeed).
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{
		{Url: alice.JoinPath("book")},
		{Url: bob.JoinPath("book")},
	}}})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	bvn := PartitionUrl("BVN0")
	synth := bvn.JoinPath(Synthetic)
	part := sim.S.Partition("BVN0")

	// The node is away for every send, so it holds none of their companions.
	part.RestartNode(joiner)
	for i := uint64(1); i <= 5; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
	}
	sim.StepN(10)

	// The companions the peer's chain names, and the node does not hold.
	companions := map[[32]byte]bool{}
	View(t, part.NodeDatabase(0), func(batch *database.Batch) {
		c := batch.Account(synth).SyntheticChain(Directory)
		head, err := c.Head().Get()
		require.NoError(t, err)
		for i := int64(0); i < head.Count; i++ {
			h, err := c.Entry(i)
			require.NoError(t, err)
			var seq *messaging.SequencedMessage
			require.NoError(t, batch.Message2(h).Main().GetAs(&seq))
			if m, ok := seq.Message.(messaging.MessageForTransaction); ok && seq.Message.Type() != messaging.MessageTypeBlockAnchor {
				companions[m.GetTxID().Hash()] = true
			}
		}
	})
	View(t, part.NodeDatabase(joiner), func(batch *database.Batch) {
		for h := range companions {
			if _, err := batch.Message(h).Main().Get(); err == nil {
				delete(companions, h)
			}
		}
	})
	require.NotEmpty(t, companions, "precondition: the peer's synthetic chain names transactions the node does not hold")

	peer := api.Querier2{Querier: sim.S.Services().ForPeer(part.NodePeerID(0)).ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr())}
	batch := part.NodeDatabase(joiner).Begin(true)
	require.NoError(t, pull.Account(context.Background(), peer, batch, synth, pull.Options{Mode: pull.ModeFullSpine}))
	require.NoError(t, batch.Commit())

	View(t, part.NodeDatabase(joiner), func(batch *database.Batch) {
		for h := range companions {
			var txn *messaging.TransactionMessage
			err := batch.Message(h).Main().GetAs(&txn)
			require.NoError(t, err, "the synthetic ledger was taken without the transaction %x a synthetic names", h[:4])
			require.Equal(t, h, *(*[32]byte)(txn.Transaction.GetHash()))
		}
	})
}

// freshBVN0Executor is an executor for BVN0 over db, as a new process builds
// one: its cache unseeded.
func freshBVN0Executor(t *testing.T, sim *Sim, db *database.Database, cache *synthcache.Cache) coreexec.Executor {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	x, err := multi.NewExecutor(coreexec.Options{
		Logger:        acctesting.NewTestLogger(t),
		Database:      db,
		Key:           priv,
		Router:        sim.S.Router(),
		EventBus:      events.NewBus(nil),
		NewDispatcher: func() coreexec.Dispatcher { return nopDispatcher{} },
		Sequencer:     sim.S.Services().Private(),
		Querier:       sim.S.Services(),
		Describe:      coreexec.DescribeShim{NetworkType: PartitionTypeBlockValidator, PartitionId: "BVN0"},
		Staging:       coreexec.NewStaging(),
		SynthCache:    cache,
	})
	require.NoError(t, err)
	return x
}
