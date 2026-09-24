// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAJoinedNodeCountsAStraddlingAnchorAsItsPeersDo — #4416 review F1. The
// executor counts an anchor's quorum from the pool's per-transaction
// ValidatorSignatures (anchor_signatures.go, anchorIsAdmissible), which the
// block that adds a copy writes. It is under the account hash only for a
// pending transaction, and an anchor below its quorum is not recorded
// pending, so a pull that does not rebuild it verifies all the same.
//
// Here every BVN0 anchor reaches the Directory with one validator's copy in
// one block and the others a few blocks later, as copies do on a live
// network, so at any block some anchor is below its quorum of two. A
// Directory node restarts and rejoins by pull at Q while that goes on. At Q
// its peers hold one signature for such an anchor; a joined node that
// rebuilt no set holds none. When the second copy arrives after Q, on every
// node alike, the peers count two and execute, and the joined node counts one
// and holds: the Directory's state diverges on a block after a good handoff.
func TestAJoinedNodeCountsAStraddlingAnchorAsItsPeersDo(t *testing.T) {
	const joiner = 1
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// The copies of BVN0's anchors to the Directory from every validator but
	// one are held back while delaying is on, and delivered by release.
	var mu sync.Mutex
	var first []byte
	var delayed []*messaging.Envelope
	delaying := false
	delay := func(_ context.Context, env *messaging.Envelope) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if !delaying {
			return true, nil
		}
		msgs, err := env.Normalize()
		if err != nil {
			return true, nil
		}
		for _, m := range msgs {
			ba, ok := m.(*messaging.BlockAnchor)
			if !ok || ba.Signature == nil {
				continue
			}
			seq, ok := ba.Anchor.(*messaging.SequencedMessage)
			if !ok || !seq.Source.Equal(PartitionUrl("BVN0")) || !seq.Destination.Equal(DnUrl()) {
				continue
			}
			if first == nil {
				first = ba.Signature.GetPublicKey()
			}
			if !bytes.Equal(first, ba.Signature.GetPublicKey()) {
				// An envelope is one validator's dispatch: delay all of it.
				delayed = append(delayed, env)
				return false, nil
			}
		}
		return true, nil
	}

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.CaptureDispatchedMessages(delay),
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

	dn := DnUrl()
	p := sim.S.Partition(Directory)
	pool := dn.JoinPath(AnchorPool)

	release := func() {
		mu.Lock()
		envs := delayed
		delayed = nil
		mu.Unlock()
		for _, env := range envs {
			_, err := p.Submit(env, false)
			require.NoError(t, err)
		}
	}
	var ts uint64
	submit := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	// One round of load: a transaction, the copies held back last round
	// delivered, and three blocks.
	round := func() {
		submit()
		release()
		sim.StepN(3)
	}
	for i := 0; i < 5; i++ {
		round()
	}

	mu.Lock()
	delaying = true
	mu.Unlock()
	r := partitionBlock(t, p.NodeDatabase(joiner), dn)
	p.RestartNode(joiner)
	for i := 0; i < 8; i++ {
		round()
	}

	// The node starts collecting again with nothing in hand, so it cannot
	// execute from R and pulls. A daemon reaches that shape through a join
	// buffer overrun (#4407), and after #4405 through any outage longer than
	// DAGGCDepth; a short outage keeps its buffer and pulls nothing (see
	// TestARollingRestartOfTheDirectoryLeavesItsAnchorsServable).
	p.RestartNode(joiner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{cancel: cancel, State: p.NodeJoinState(joiner), step: func(n int) {
		round()
		if n >= 200 {
			cancel()
		}
	}}
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: Directory,
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: Directory, Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.False(t, p.Joining(joiner))
	q := partitionBlock(t, p.NodeDatabase(joiner), dn)
	t.Logf("the Directory node stopped at R=%d and joined at Q=%d", r, q)
	var pulled int
	View(t, p.NodeDatabase(joiner), func(batch *database.Batch) {
		head, err := batch.Account(pool).MainChain().Head().Get()
		require.NoError(t, err)
		for i := int64(0); i < head.Count; i++ {
			h, err := batch.Account(pool).MainChain().Entry(i)
			require.NoError(t, err)
			st, err := batch.Transaction(h).Status().Get()
			require.NoError(t, err)
			if !st.Delivered() {
				pulled++
			}
		}
	})
	require.NotZero(t, pulled, "premise: the node holds anchors only by pull")

	// Every copy still held back arrives after the join, on every node alike.
	mu.Lock()
	delaying = false
	mu.Unlock()
	for i := 0; i < 6; i++ {
		round()
	}

	// What each node counts an anchor's quorum from, for every anchor with a
	// signature on the pool's signature chain.
	counts := func(node int) map[[32]byte]int {
		out := map[[32]byte]int{}
		View(t, p.NodeDatabase(node), func(batch *database.Batch) {
			head, err := batch.Account(pool).SignatureChain().Head().Get()
			require.NoError(t, err)
			for i := int64(0); i < head.Count; i++ {
				h, err := batch.Account(pool).SignatureChain().Entry(i)
				require.NoError(t, err)
				var ba *messaging.BlockAnchor
				if batch.Message2(h).Main().GetAs(&ba) != nil {
					continue
				}
				txh := ba.Anchor.(*messaging.SequencedMessage).Message.ID().Hash()
				sigs, err := batch.Account(pool).Transaction(txh).ValidatorSignatures().Get()
				require.NoError(t, err)
				out[txh] = len(sigs)
			}
		})
		return out
	}
	peer, joined := counts(0), counts(joiner)
	var differ int
	for h, n := range peer {
		if joined[h] != n {
			differ++
		}
	}
	t.Logf("%d of %d anchors on the pool are counted from another set on the joined node", differ, len(peer))
	require.Zero(t, differ, "the joined node counts anchors' signatures from other sets than its peers")
	require.Equal(t, bptRoot(t, p.NodeDatabase(0)), bptRoot(t, p.NodeDatabase(joiner)),
		"the joined node's state diverged from its peers' once the second copies arrived")
}
