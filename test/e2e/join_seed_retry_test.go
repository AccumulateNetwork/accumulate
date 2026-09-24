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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multi "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// A new process seeds its producer cache once, at the first block it opens
// (block_begin.go). A seed that failed used the once up: the block after it
// opened with no seed and an empty cache, and nothing said so -- in run
// 20260924T052134Z every group after the first failed further down, at
// lastAnchoredBlock, because the cache that should have answered was empty
// (#4400). A seed that fails is tried again at the next block, and only one
// that succeeds counts.
func TestAFailedSeedIsTriedAgainAtTheNextBlock(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(20)

	dn := DnUrl()
	p := sim.S.Partition("Directory")
	db := p.NodeDatabase(0)
	next := partitionBlock(t, db, dn) + 1

	// An entry on the anchor pool's main chain with nothing behind it: what a
	// join that took the chain as hashes left (#4400).
	hole := [32]byte{0x44, 0x00}
	func() {
		batch := db.Begin(true)
		defer batch.Discard()
		require.NoError(t, batch.Account(dn.JoinPath(AnchorPool)).MainChain().Inner().AddEntry(hole[:], false))
		require.NoError(t, batch.Commit())
	}()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cache := synthcache.New(0)
	x, err := multi.NewExecutor(coreexec.Options{
		Logger:        acctesting.NewTestLogger(t),
		Database:      db,
		Key:           priv,
		Router:        sim.S.Router(),
		EventBus:      events.NewBus(nil),
		NewDispatcher: func() coreexec.Dispatcher { return nopDispatcher{} },
		Sequencer:     sim.S.Services().Private(),
		Querier:       sim.S.Services(),
		Describe:      coreexec.DescribeShim{NetworkType: PartitionTypeDirectory, PartitionId: Directory},
		Staging:       coreexec.NewStaging(),
		SynthCache:    cache,
	})
	require.NoError(t, err)

	_, err = x.Begin(coreexec.BlockParams{Context: context.Background(), Index: next, Time: time.Now()})
	require.ErrorContains(t, err, "seed synthetic cache", "precondition: the seed fails on an entry with no message")
	_, _, seeded := cache.PeekAnchor(1)
	require.False(t, seeded, "precondition: a seed that failed seeded nothing")

	// The message arrives (a peer served it); the next block's open seeds.
	func() {
		batch := db.Begin(true)
		defer batch.Discard()
		txn := new(Transaction)
		txn.Header.Principal = dn.JoinPath(AnchorPool)
		txn.Body = &WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{{1}}}}
		require.NoError(t, batch.Message(hole).Main().Put(&messaging.TransactionMessage{Transaction: txn}))
		require.NoError(t, batch.Commit())
	}()

	_, err = x.Begin(coreexec.BlockParams{Context: context.Background(), Index: next, Time: time.Now()})
	require.NoError(t, err)
	_, _, seeded = cache.PeekAnchor(1)
	require.True(t, seeded, "a seed that failed was not tried again: the block opened with the cache it would have had if the seed had never run")
}
