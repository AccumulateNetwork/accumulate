// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// A restarted node's cache holds every own block the Directory has not
// receipted — nothing from them was dispatched — and the in-flight tail below
// the newest receipt, marked dispatched so healing can be answered from it
// (#4241). An older block is held only for an entry its destination has not
// delivered, and then it was receipted and is dispatched (#4464): no
// destination has delivered anything this node has recorded here, so every
// produced block is owed. Here the Directory's anchors stop executing on BVN0
// while it keeps producing, so its newest receipt falls well behind.
func TestSeedHoldsWhatTheDirectoryHasNotReceipted(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	var mu sync.Mutex
	stores := map[string]keyvalue.Beginner{}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.Genesis(GenesisTime),
		simulator.WithDatabase(func(p *PartitionInfo, node int, _ logging.Logger) keyvalue.Beginner {
			s := memory.New(nil)
			mu.Lock()
			defer mu.Unlock()
			if node == 0 {
				stores[p.ID] = s
			}
			return s
		}),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	// Directory anchors stop executing on BVN0 once the switch is thrown,
	// however they arrive
	var dropAnchors atomic.Bool
	sim.S.SetBlockHook("BVN0", func(_ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		if !dropAnchors.Load() {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			drop := false
			for _, m := range env.Messages {
				blk, ok := m.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if seq, ok := blk.Anchor.(*messaging.SequencedMessage); ok {
					if txn, ok := seq.Message.(*messaging.TransactionMessage); ok && txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
						drop = true
					}
				}
			}
			if !drop {
				kept = append(kept, env)
			}
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}

	// Steady traffic with the Directory answering
	for i := uint64(1); i <= 20; i++ {
		send(i)
		sim.Step()
	}
	sim.StepN(10)

	// Then the Directory's anchors stop reaching BVN0's blocks while it keeps
	// producing
	dropAnchors.Store(true)
	for i := uint64(21); i <= 50; i++ {
		send(i)
		sim.Step()
	}

	// A restarted BVN0: a fresh cache seeded from its store
	db := database.New(stores["BVN0"], nil)
	cache := synthcache.New(0)
	probe, err := block.NewSeedProbe(execute.DescribeShim{NetworkType: PartitionTypeBlockValidator, PartitionId: "BVN0"}, db, cache)
	require.NoError(t, err)
	current := sim.S.BlockIndex("BVN0") + 1
	batch := db.Begin(false)
	defer batch.Discard()
	require.NoError(t, probe.SeedSynthCache(batch, current))

	// The newest receipted own block is the newest dispatched block held
	var newest uint64
	var held []uint64
	for b := uint64(1); b < current; b++ {
		blk, ok := cache.Block(b)
		if !ok {
			continue
		}
		held = append(held, b)
		if blk.Dispatched && b > newest {
			newest = b
		}
	}
	t.Logf("current %d, newest receipted %d, %d blocks held", current, newest, len(held))
	require.NotZero(t, newest, "the Directory receipted blocks before its anchors were dropped")
	require.Less(t, newest+synthcache.InFlightBlocks, current-1, "precondition: the Directory lags by more than the in-flight tail")

	for _, b := range held {
		blk, _ := cache.Block(b)
		if b < newest-synthcache.InFlightBlocks {
			require.NotEmpty(t, blk.Entries, "block %d is older than the in-flight tail and holds nothing owed", b)
		}
		if b <= newest {
			require.True(t, blk.Dispatched, "block %d was receipted, so it was dispatched", b)
		} else {
			require.False(t, blk.Dispatched, "block %d was not receipted, so it was not dispatched", b)
		}
	}

	// Every block above the newest receipt is held, with what it produced
	var entries int
	for b := newest + 1; b < current; b++ {
		blk, ok := cache.Block(b)
		require.True(t, ok, "block %d is not receipted and must be held", b)
		entries += len(blk.Entries)
	}
	require.Greater(t, entries, 0, "the unreceipted blocks produced synthetics")
}

// TestTheSeedReadsOnlyWhatAJoinTook — #4452. A follower's join takes the
// anchor pool, the ledger and the synthetic ledger by their recent window and
// backfills the rest after the match. The first block it opens seeds the
// producer's cache from them, and with the Directory lagging the seed reaches
// back past the window: every own block since the newest receipt, up to the
// horizon. What it reaches that the node does not hold is a block the node
// did not execute, and the walks of the pool stop there; none of it fails
// the seed, which would fail every handoff.
func TestTheSeedReadsOnlyWhatAJoinTook(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	var mu sync.Mutex
	stores := map[string]keyvalue.Beginner{}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.Genesis(GenesisTime),
		simulator.WithDatabase(func(p *PartitionInfo, node int, _ logging.Logger) keyvalue.Beginner {
			s := memory.New(nil)
			mu.Lock()
			defer mu.Unlock()
			if node == 0 {
				stores[p.ID] = s
			}
			return s
		}),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	// Directory anchors stop executing on BVN0 once the switch is thrown,
	// however they arrive
	var dropAnchors atomic.Bool
	sim.S.SetBlockHook("BVN0", func(_ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		if !dropAnchors.Load() {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			drop := false
			for _, m := range env.Messages {
				blk, ok := m.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if seq, ok := blk.Anchor.(*messaging.SequencedMessage); ok {
					if txn, ok := seq.Message.(*messaging.TransactionMessage); ok && txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
						drop = true
					}
				}
			}
			if !drop {
				kept = append(kept, env)
			}
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}

	// Steady traffic with the Directory answering
	for i := uint64(1); i <= 20; i++ {
		send(i)
		sim.Step()
	}
	sim.StepN(10)

	// Then the Directory's anchors stop reaching BVN0's blocks while it keeps
	// producing
	dropAnchors.Store(true)
	for i := uint64(21); i <= 400; i++ {
		send(i)
		sim.Step()
	}

	// A follower's store as its join leaves it: the network's state, and
	// the three accounts by the least window a join takes -- each chain's
	// last mark set.
	node := database.New(stores["BVN0"], nil)
	src := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: node, Partition: "BVN0"})}
	db := database.OpenInMemory(nil)
	bvn := PartitionUrl("BVN0")
	batch := db.Begin(true)
	for _, u := range []*url.URL{bvn.JoinPath(Network), bvn.JoinPath(Globals), bvn.JoinPath(Oracle), bvn.JoinPath(Routing), bvn.JoinPath(Operators), bvn.JoinPath(Operators, "1")} {
		require.NoError(t, pull.Account(context.Background(), src, batch, u, pull.Options{Mode: pull.ModeStateOnly}), "%v", u)
	}
	for _, u := range []*url.URL{bvn.JoinPath(AnchorPool), bvn.JoinPath(Ledger), bvn.JoinPath(Synthetic)} {
		require.NoError(t, pull.Account(context.Background(), src, batch, u, pull.Options{Mode: pull.ModeWindow}), "%v", u)
	}
	require.NoError(t, batch.Commit())
	func() {
		b := db.Begin(false)
		defer b.Discard()
		_, err := b.Account(bvn.JoinPath(Synthetic)).SyntheticChain("BVN1").Index().Inner().Entry(0)
		require.Error(t, err, "precondition: the window holds the synthetic index chain's first entry")
	}()

	cache := synthcache.New(0)
	probe, err := block.NewSeedProbe(execute.DescribeShim{NetworkType: PartitionTypeBlockValidator, PartitionId: "BVN0"}, db, cache)
	require.NoError(t, err)
	current := sim.S.BlockIndex("BVN0") + 1
	view := db.Begin(false)
	defer view.Discard()
	require.NoError(t, probe.SeedSynthCache(view, current), "the seed failed on what the window left")
	// What the windows hold whole is rebuilt from the peers' entries, as the
	// node would have built it; what they do not is skipped.
	var rebuilt int
	for b := uint64(1); b < current; b++ {
		if _, ok := cache.Block(b); ok {
			rebuilt++
		}
	}
	t.Logf("current %d: %d blocks rebuilt from the windows", current, rebuilt)
	require.Less(t, rebuilt, int(current-1), "precondition: the seed reached past the windows")
}
