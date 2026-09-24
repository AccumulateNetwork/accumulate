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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The retry path the handoff takes since #4401: attempt 1 fails at the seed
// (no peer held the message), the node goes back to collecting, and attempt 2
// opens the same block again. That second open must seed rather than pass
// unseeded, and the first must not leak its batch (#4398 review F1,
// note_3896127189).
//
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
	opened := &batchesOpened{Beginner: db}
	x, err := multi.NewExecutor(coreexec.Options{
		Logger:        acctesting.NewTestLogger(t),
		Database:      opened,
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

	// The failed open's write batch is discarded, not leaked: a discarded
	// batch panics when it is used again, and a leaked one commits
	// (#4398 review F1).
	require.NotEmpty(t, opened.writable, "precondition: the open began a write batch")
	for i, b := range opened.writable {
		require.Panics(t, func() { _ = b.Commit() }, "write batch %d of the failed open was left open", i)
	}
	opened.writable = nil

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

// batchesOpened records the write batches an executor begins.
type batchesOpened struct {
	database.Beginner
	writable []*database.Batch
}

func (d *batchesOpened) Begin(writable bool) *database.Batch {
	b := d.Beginner.Begin(writable)
	if writable {
		d.writable = append(d.writable, b)
	}
	return b
}
