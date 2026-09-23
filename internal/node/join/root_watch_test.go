// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join_test

import (
	"context"
	"math/big"
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

// A joined node whose state goes wrong after the handoff is caught by the
// root check and syncs again, through the production wiring: join.Run with
// the simulator's node as its buffer, the executor's stage, and a
// join.PulledState over join.QueryPeers with the node's own peer ID excluded
// (#4303) — what cmd/accumulated/run/dagbft.go hands it. Nothing is copied
// from a peer by the test.
//
// The wrong state is fault injection: one account in the joined node's store
// is changed behind the executor's back, as a missed gap would change it. The
// node goes on executing blocks from that state, and the first anchored root
// its bpt chain disagrees with is where the join must stop it, collect again,
// pull, and hand off a second time. It ends on the root chain its peers are
// on.
func TestJoin_AnExecutedBlockWhoseRootDivergesIsSyncedAgainThroughTheProductionPull(t *testing.T) {
	const joiner = 2

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
	submit := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 5; i++ {
		submit()
		sim.StepN(3)
	}
	sim.StepN(10)

	part := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	p.RestartNode(joiner)
	for i := 0; i < 3; i++ {
		submit()
		sim.StepN(3)
	}

	pulled, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  p.NodeDatabase(joiner),
		Sources: &join.QueryPeers{
			Client:  sim.S.Services(),
			Network: t.Name(),
			Router:  sim.S.Router(),
			Self:    p.NodePeerID(joiner),
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// The simulator advances only when something steps it, and a real
	// network runs on while a node pulls and while it executes: the state
	// steps it on every pull and every root check, with a transaction every
	// few blocks so the blocks carry something and are anchored.
	buf := &countingBuffer{Buffer: p.NodeJoin(joiner)}
	var corruptedAt, checksAfterSecond int
	watches := 0
	state := &steppingPulledState{PulledState: pulled}
	state.step = func() {
		if state.steps%4 == 0 {
			submit()
		}
		state.steps++
		sim.Step()
	}
	state.watch = func() {
		watches++
		switch {
		case len(buf.handoffs) == 1 && corruptedAt == 0 && watches == 20:
			// Twenty root checks on a correct state have not stopped the
			// node: a check that is not there, or one that fires on a good
			// state, is caught here rather than passed off as a re-sync.
			require.Equal(t, 1, buf.starts, "precondition: the node was not stopped while its state was right")
			corrupt(t, p.NodeDatabase(joiner), bob.JoinPath("tokens"))
			corruptedAt = watches
		case len(buf.handoffs) > 1:
			checksAfterSecond++
			if checksAfterSecond == 20 {
				cancel()
			}
		case watches > 400:
			cancel()
		}
	}

	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err = join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     state,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)

	require.NotEmpty(t, buf.handoffs, "the join hands off")
	require.NotZero(t, corruptedAt, "precondition: the joined node's state was corrupted after the handoff")
	require.Equal(t, 2, buf.starts, "the root check stops the node and it collects again")
	require.Len(t, buf.handoffs, 2, "and it hands off a second time, from the state it pulled")
	require.Equal(t, 20, checksAfterSecond, "and the roots it executes from there match")
	require.False(t, p.Joining(joiner), "the node is executing")

	sim.StepN(10)
	var anchors [][]byte
	for i := 0; i < p.NodeCount(); i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "node %d's root chain is not node 0's after the re-sync", i)
	}
}

// corrupt changes an account in one node's store, and its BPT leaf with it,
// without a block: the fault a missed gap leaves.
func corrupt(t *testing.T, db *database.Database, account *url.URL) {
	t.Helper()
	Update(t, db, func(batch *database.Batch) {
		var tokens *TokenAccount
		require.NoError(t, batch.Account(account).Main().GetAs(&tokens))
		tokens.Balance.Add(&tokens.Balance, big.NewInt(7))
		require.NoError(t, batch.Account(account).Main().Put(tokens))
		require.NoError(t, batch.UpdateBPT())
	})
}

// steppingPulledState is the production state with the simulator stepped on
// every pull and every root check. It embeds the concrete *join.PulledState,
// so Run sees its Diverged and HandedOff as the daemon's join does.
type steppingPulledState struct {
	*join.PulledState
	steps int
	step  func()
	watch func()
}

func (s *steppingPulledState) Pull(ctx context.Context) error {
	err := s.PulledState.Pull(ctx)
	s.step()
	return err
}

func (s *steppingPulledState) Diverged(ctx context.Context) (uint64, bool, error) {
	s.step()
	s.watch()
	return s.PulledState.Diverged(ctx)
}

// countingBuffer is the simulator's buffer, counted.
type countingBuffer struct {
	join.Buffer
	starts   int
	handoffs []uint64
}

func (b *countingBuffer) StartCollecting() {
	b.starts++
	b.Buffer.StartCollecting()
}

func (b *countingBuffer) Handoff(q uint64) error {
	err := b.Buffer.Handoff(q)
	if err == nil {
		b.handoffs = append(b.handoffs, q)
	}
	return err
}
