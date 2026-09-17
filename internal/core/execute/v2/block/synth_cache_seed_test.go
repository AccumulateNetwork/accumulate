// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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
// the newest receipt, marked dispatched so healing can be answered from it;
// nothing older (#4241). Here the Directory's anchors stop executing on BVN0
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
		require.GreaterOrEqual(t, b, newest-synthcache.InFlightBlocks, "block %d is older than the in-flight tail below the newest receipt", b)
		blk, _ := cache.Block(b)
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

// A re-seeded block must answer for its entries the way the live block did:
// its continuation receipt -- the root receipt from the stream's anchor to
// the block's root, combined with the Directory's receipt for that block --
// must chain. On run 20260917T212457Z every heal request that landed on a
// freshly restarted validator failed with "continue receipt list: receipts
// cannot be combined", 115 of them on the rerun, and requesters had to try
// another node (#4287).
//
// This passes: one stream, one entry a block, receipts landed before the
// restart, the seeded continuation chains for every receipted block. So the
// production failure needs something this does not do -- several
// destinations anchoring in one block, packages of several entries a block,
// a major block inside the seeded window, or a Directory receipt that arrives
// after the restart and is attached to a rebuilt block. It stays as the
// fixture those cases are added to, and as the guard against the simple case
// regressing.
func TestSeedSynthCache_ContinuationCombines(t *testing.T) {
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

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Steady cross-partition traffic with the Directory answering, right up
	// to the restart: the seed holds only the in-flight tail below the newest
	// Directory receipt, so the blocks it rebuilds must be ones that produced
	// synthetics and were receipted -- traffic that stopped earlier leaves
	// the seed nothing but empty blocks, which carry no receipt to chain.
	for i := uint64(1); i <= 60; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
		sim.Step()
	}
	sim.StepN(4)

	// A restarted BVN0: a fresh cache seeded from its store
	db := database.New(stores["BVN0"], nil)
	cache := synthcache.New(0)
	probe, err := block.NewSeedProbe(execute.DescribeShim{NetworkType: PartitionTypeBlockValidator, PartitionId: "BVN0"}, db, cache)
	require.NoError(t, err)
	current := sim.S.BlockIndex("BVN0") + 1
	batch := db.Begin(false)
	defer batch.Discard()
	require.NoError(t, probe.SeedSynthCache(batch, current))

	receipted, chained := 0, 0
	var failures []string
	for b := uint64(1); b < current; b++ {
		blk, ok := cache.Block(b)
		if !ok {
			continue
		}
		roots := 0
		for _, st := range blk.Streams {
			if st.RootReceipt != nil {
				roots++
			}
		}
		t.Logf("seeded block %d: dispatched=%v directoryReceipt=%v streams=%d rootReceipts=%d entries=%d",
			b, blk.Dispatched, blk.DirectoryReceipt != nil, len(blk.Streams), roots, len(blk.Entries))
		if !blk.Dispatched || blk.DirectoryReceipt == nil {
			continue
		}
		for _, st := range blk.Streams {
			if st.RootReceipt == nil {
				continue
			}
			receipted++
			r, err := blk.Continuation(st.Destination)
			if err != nil {
				failures = append(failures, fmt.Sprintf("block %d -> %v: %v", b, st.Destination, err))
				continue
			}
			require.NotNil(t, r, "block %d -> %v: a receipted block continues", b, st.Destination)
			chained++
		}
	}
	t.Logf("current %d: %d receipted streams seeded, %d chain", current, receipted, chained)
	require.NotZero(t, receipted, "precondition: the Directory receipted seeded blocks")
	require.Empty(t, failures, "a re-seeded block's continuation receipt must chain (#4287)")
}
