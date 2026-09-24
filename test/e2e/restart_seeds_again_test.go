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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// unreadableAtFirstHandoff makes the message behind the newest entry of the
// pool's main chain one the seed cannot read, until the first handoff that
// tries to produce a block -- one that does not only wait (NotReady) or find
// the state behind (Conflict) -- and records what that handoff and every one
// after it answered.
type unreadableAtFirstHandoff struct {
	join.Buffer
	db   *database.Database
	pool *url.URL
	errs []error
}

func (b *unreadableAtFirstHandoff) Handoff(q uint64) error {
	if len(b.errs) > 0 {
		err := b.Buffer.Handoff(q)
		b.errs = append(b.errs, err)
		return err
	}

	var key [32]byte
	var held messaging.Message
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(b.db.Update(func(batch *database.Batch) error {
		c := batch.Account(b.pool).MainChain()
		head, err := c.Head().Get()
		if err != nil {
			return err
		}
		h, err := c.Entry(head.Count - 1)
		if err != nil {
			return err
		}
		key = *(*[32]byte)(h)
		held, err = batch.Message(key).Main().Get()
		if err != nil {
			return err
		}
		return batch.Message(key).Main().Put(&messaging.SignatureMessage{Signature: &ED25519Signature{}})
	}))
	err := b.Buffer.Handoff(q)
	must(b.db.Update(func(batch *database.Batch) error {
		return batch.Message(key).Main().Put(held)
	}))
	if err == nil || !errors.Is(err, errors.NotReady) && !errors.Is(err, errors.Conflict) {
		b.errs = append(b.errs, err)
	}
	return err
}

// TestARestartedSimulatorNodeSeedsAtItsFirstBlock — #4421. The first block a
// new process opens seeds the producer cache from the store, reading the
// message behind the anchor pool's newest entries. The simulator keeps one
// executor per node across RestartNode, and its "seeded" latch survived, so a
// validator that had executed and then restarted never seeded again: a join
// that left the pool unreadable handed off anyway, where the daemon fails
// its first block. Here the pool is unreadable at the first handoff, so the
// restarted validator must fail it, as a new process would, and hand off at
// the next.
func TestARestartedSimulatorNodeSeedsAtItsFirstBlock(t *testing.T) {
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
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// The validator executes, so its executor has seeded, and then restarts.
	const node = 1
	part := sim.S.Partition("BVN0")
	for i := 0; i < 5; i++ {
		send()
	}
	sim.StepN(10)
	part.RestartNode(node)
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{State: part.NodeJoinState(node), step: func(round int) {
		sim.StepN(3)
		if round >= 200 {
			cancel()
		}
	}}
	buffer := &unreadableAtFirstHandoff{Buffer: part.NodeJoin(node), db: part.NodeDatabase(node), pool: PartitionUrl("BVN0").JoinPath(AnchorPool)}
	settler, ok := part.NodeExecutor(node).(join.Settler)
	require.True(t, ok)
	outcome, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buffer,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: part.NodeStaging(node), Database: part.NodeDatabase(node)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, join.Joined, outcome)

	require.NotEmpty(t, buffer.errs)
	first := buffer.errs[0]
	require.Error(t, first, "the restarted validator opened its first block without seeding: the simulator's restart kept the executor's seed latch")
	require.True(t, strings.Contains(first.Error(), "seed synthetic cache"), "the first handoff failed, but not in the seed: %v", first)
	require.NoError(t, buffer.errs[len(buffer.errs)-1])
}
