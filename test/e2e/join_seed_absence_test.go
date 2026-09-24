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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multi "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
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

// The seed skips a block only on the store's evidence that this node did not
// execute it: a synthetic entry with no message behind it. Any other absence
// in a block the node executed -- here the companion transaction of one of
// its synthetics -- is a broken store, not a join, and fails the seed loudly
// rather than being skipped. Matching that evidence by status code made every
// NotFound in the rebuild a silent skip (#4400 re-check, note_3896258110).
func TestAnExecutedBlockMissingACompanionFailsTheSeed(t *testing.T) {
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
	// Directory: every transaction on them sends a signature request there,
	// a synthetic whose companion is the transaction it names.
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{
		{Url: alice.JoinPath("book")},
		{Url: bob.JoinPath("book")},
	}}})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	for i := uint64(1); i <= 5; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
	}
	sim.StepN(3)

	bvn := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	db := p.NodeDatabase(0)
	h := partitionBlock(t, db, bvn)

	fresh := func(cache *synthcache.Cache) coreexec.Executor {
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

	// A synthetic this node produced, and its companion transaction.
	var entry, companion [32]byte
	View(t, db, func(batch *database.Batch) {
		c := batch.Account(bvn.JoinPath(Synthetic)).SyntheticChain(Directory)
		head, err := c.Head().Get()
		require.NoError(t, err)
		require.NotZero(t, head.Count, "precondition: BVN0 produced synthetics for the Directory")
		// The newest one that has a companion, as the seed reads it.
		for i := head.Count - 1; i >= 0 && companion == ([32]byte{}); i-- {
			hash, err := c.Entry(i)
			require.NoError(t, err)
			var seq *messaging.SequencedMessage
			require.NoError(t, batch.Message2(hash).Main().GetAs(&seq))
			msg, ok := seq.Message.(messaging.MessageForTransaction)
			if !ok || seq.Message.Type() == messaging.MessageTypeBlockAnchor {
				continue
			}
			entry = *(*[32]byte)(hash)
			companion = msg.GetTxID().Hash()
		}
		require.NotZero(t, companion, "precondition: a synthetic with a companion transaction")
	})

	// Control: the seed rebuilds that synthetic.
	control := synthcache.New(0)
	_, err := fresh(control).Begin(coreexec.BlockParams{Context: context.Background(), Index: h + 1, Time: time.Now()})
	require.NoError(t, err)
	_, held := control.ByHash(entry)
	require.True(t, held, "precondition: the seed rebuilds the block that produced the newest synthetic")

	// The companion transaction is gone from the store; the synthetic's own
	// message is still there, so this is a block the node executed.
	func() {
		batch := db.Begin(false)
		key := batch.Message(companion).Main().Key()
		batch.Discard()
		kv, err := db.Store()
		require.NoError(t, err)
		cs := kv.Begin(nil, true)
		require.NoError(t, cs.Delete(key))
		require.NoError(t, cs.Commit())
	}()

	cache := synthcache.New(0)
	_, err = fresh(cache).Begin(coreexec.BlockParams{Context: context.Background(), Index: h + 1, Time: time.Now()})
	require.Error(t, err, "a block this node executed, missing a companion transaction, was skipped as one it did not execute")
	require.ErrorContains(t, err, "load transaction for synthetic message")
}
