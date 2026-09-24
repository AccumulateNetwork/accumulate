// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAFreshBVNNodeJoinsByPull — #4421. A BVN0 node with a genesis store
// joins a running network by the production join.Run and hands off.
//
// On issue-4205-lead @ b67bf3b21 it pulled and then failed every handoff:
//
//	produce buffered block N: begin block: seed synthetic cache: load
//	Directory receipts: load anchor pool main chain entry K: Message.… not found
//
// The fresh node took bvn-BVN0.acme/anchors in full (with the message behind
// every entry) in the spine pass. Every later pass names the pool again -- the
// pool is written every block, so the block ledger names it -- and fetchPass
// skipped the spine accounts only in the spine pass, so the later passes pulled
// the pool STATE-ONLY: chain heads and the open mark set's entries, no message
// behind any of them. The pool's main chain then held entries past the spine
// pass with no message, and the seed reads the message behind the newest
// (ownReceipts, internal/core/execute/v2/block/synth_cache_seed.go).
func TestAFreshBVNNodeJoinsByPull(t *testing.T) {
	freshNodeJoinsByPull(t, "BVN0", nil)
}

// TestAFreshDirectoryNodeJoinsByPull — #4421. The same for the Directory: it
// failed 2 of 2 on the lead (dn.acme/anchors at 409 entries, 384 without a
// message). #4416's rolling-restart test joined a fresh Directory node only
// because that join happened to hand off in the spine pass.
func TestAFreshDirectoryNodeJoinsByPull(t *testing.T) {
	freshNodeJoinsByPull(t, Directory, nil)
}

// TestANodeHoldingPoolEntriesWithNoMessageJoins — #4421. A node that joined
// on the lead before the fix holds its pool's entries past its first pass
// with no message behind them, and a full pull resumes from the local head,
// so it brings nothing back for them: the node's seed would fail at every
// process start. The store here is written that way, by the pull itself --
// the pool taken whole, the network moved on, the pool taken state-only --
// and the join must fetch the missing messages and hand off.
func TestANodeHoldingPoolEntriesWithNoMessageJoins(t *testing.T) {
	freshNodeJoinsByPull(t, "BVN0", func(sim *Sim, part *simulator.Partition, node int, send func()) {
		pool := PartitionUrl("BVN0").JoinPath(AnchorPool)
		peer := api.Querier2{Querier: sim.S.Services().ForPeer(part.NodePeerID(0)).ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr())}
		take := func(mode pull.Mode) {
			batch := part.NodeDatabase(node).Begin(true)
			defer batch.Discard()
			require.NoError(t, pull.Account(context.Background(), peer, batch, pool, pull.Options{Mode: mode}))
			require.NoError(t, batch.Commit())
		}
		take(pull.ModeFullSpine)
		for i := 0; i < 3; i++ {
			send()
		}
		sim.StepN(10)
		take(pull.ModeStateOnly)
		require.NotEmpty(t, entriesWithNoMessage(t, part.NodeDatabase(node), "BVN0")[strings.ToLower(pool.String())+"#main"],
			"precondition: the store holds pool entries with no message behind them")
	})
}

// freshNodeJoinsByPull stands the follower's node of the partition on a
// genesis store, lets the network run, drops what the node collected (a
// process start), optionally damages its store, and runs the production join
// under load. The node must hand off, and hold no entry of a spine account's
// transaction chain without the message behind it.
func freshNodeJoinsByPull(t *testing.T, partition string, damage func(sim *Sim, part *simulator.Partition, node int, send func())) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	net, _ := networkWithAFollower(t.Name(), 1, 3)
	sim := NewSim(t,
		simulator.WithNetwork(net),
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

	part := sim.S.Partition(partition)
	const fresh = 3 // networkWithAFollower appends it after the validators
	require.Equal(t, 4, part.NodeCount())

	// The fresh node: it starts its join before any block, so it has
	// executed nothing beyond genesis.
	part.RestartNode(fresh)

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := 0; i < 5; i++ {
		send()
	}
	sim.StepN(10)
	if damage != nil {
		damage(sim, part, fresh, send)
	}

	// A process start: what the node collected is dropped, so it must pull.
	part.RestartNode(fresh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 200
	stepping := &steppingState{State: part.NodeJoinState(fresh), step: func(round int) {
		if round%5 == 0 {
			send()
		}
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}
	settler, ok := part.NodeExecutor(fresh).(join.Settler)
	require.True(t, ok)
	buffer := &handoffCounting{Buffer: part.NodeJoin(fresh)}
	outcome, err := join.Run(ctx, join.Options{
		Partition: partition,
		Buffer:    buffer,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: part.NodeStaging(fresh), Database: part.NodeDatabase(fresh)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: partition, Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	missing := entriesWithNoMessage(t, part.NodeDatabase(fresh), partition)
	require.NoError(t, err, "a fresh %s node did not join within %d pull rounds", partition, maxRounds)
	require.Equal(t, join.Joined, outcome)
	require.False(t, part.Joining(fresh))
	// The handoff's first block opens the seed, which reads the pool's newest
	// entries and the messages behind them: a store holding an entry without
	// its message fails it, and the join syncs and hands off again.
	require.Empty(t, buffer.failed, "a handoff failed to produce a block")

	// The handoff executes blocks, and the first opens the seed. Run on so
	// the node executes past it.
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(10)
	require.Empty(t, missing, "the joined node holds spine entries with no message behind them")
	require.Empty(t, entriesWithNoMessage(t, part.NodeDatabase(fresh), partition),
		"after executing, the node holds spine entries with no message behind them")
	require.Empty(t, anchorsNotRecordedAsByAValidator(t, part.NodeDatabase(fresh), part.NodeDatabase(0), partition),
		"the joined node holds pool anchors without what executing them wrote (#4416)")
}

// anchorsNotRecordedAsByAValidator is the other half of the invariant: for
// every entry of the partition's anchor pool signature chain that is an
// anchor's signature, what executing it wrote beside the entry -- the
// transaction's history index into the chain, its validator signature set,
// and its cause -- is on the node as it is on a validator that executed it
// (#4416). A hash held with its message and without these is served with no
// signatures (#4413). Keyed by the entry's position, with what differs.
func anchorsNotRecordedAsByAValidator(t *testing.T, node, validator *database.Database, partition string) map[int64]string {
	t.Helper()
	pool := PartitionUrl(partition).JoinPath(AnchorPool)
	out := map[int64]string{}
	n := node.Begin(false)
	defer n.Discard()
	v := validator.Begin(false)
	defer v.Discard()

	c := n.Account(pool).SignatureChain()
	head, err := c.Head().Get()
	require.NoError(t, err)
	for i := int64(0); i < head.Count; i++ {
		h, err := c.Entry(i)
		if err != nil {
			out[i] = "not held"
			continue
		}
		var ba *messaging.BlockAnchor
		if n.Message2(h).Main().GetAs(&ba) != nil {
			continue // not an anchor's signature, or no message (the first half)
		}
		seq, ok := ba.Anchor.(*messaging.SequencedMessage)
		if !ok {
			continue
		}
		txh := seq.Message.ID().Hash()

		nh, err := n.Account(pool).Transaction(txh).History().Get()
		require.NoError(t, err)
		vh, err := v.Account(pool).Transaction(txh).History().Get()
		require.NoError(t, err)
		if fmt.Sprint(nh) != fmt.Sprint(vh) {
			out[i] = fmt.Sprintf("history %v, the validator's %v", nh, vh)
			continue
		}
		ns, err := n.Account(pool).Transaction(txh).ValidatorSignatures().Get()
		require.NoError(t, err)
		vs, err := v.Account(pool).Transaction(txh).ValidatorSignatures().Get()
		require.NoError(t, err)
		same := len(ns) == len(vs)
		for j := 0; same && j < len(ns); j++ {
			same = EqualKeySignature(ns[j], vs[j])
		}
		if !same {
			out[i] = fmt.Sprintf("%d validator signatures, the validator's %d", len(ns), len(vs))
			continue
		}
		nc, err := n.Message(txh).Cause().Get()
		require.NoError(t, err)
		vc, err := v.Message(txh).Cause().Get()
		require.NoError(t, err)
		if fmt.Sprint(nc) != fmt.Sprint(vc) {
			out[i] = fmt.Sprintf("cause %v, the validator's %v", nc, vc)
		}
	}
	for k, v := range out {
		t.Logf("%v signature entry %d: %s", pool, k, v)
	}
	return out
}

// handoffCounting counts the join's handoffs that failed to produce a block:
// not the waits (NotReady) or the state being behind (Conflict), which a join
// meets in the ordinary way, but a group that could not be produced (#4401).
type handoffCounting struct {
	join.Buffer
	failed []error
}

func (b *handoffCounting) Handoff(q uint64) error {
	err := b.Buffer.Handoff(q)
	if err != nil && !errors.Is(err, errors.NotReady) && !errors.Is(err, errors.Conflict) {
		b.failed = append(b.failed, err)
	}
	return err
}

// entriesWithNoMessage is the store invariant #4421 broke (executor.md,
// "Sync" §3: a hash is never kept without its message): for every chain of
// every spine account of the partition whose entries are the hashes of
// messages, the positions held with no message behind them, keyed
// "<account>#<chain>". A position the store does not hold at all counts too.
func entriesWithNoMessage(t *testing.T, db *database.Database, partition string) map[string][]int64 {
	t.Helper()
	out := map[string][]int64{}
	View(t, db, func(batch *database.Batch) {
		for _, u := range pull.SpineAccounts(PartitionUrl(partition)) {
			chains, err := batch.Account(u).Chains().Get()
			require.NoError(t, err)
			for _, meta := range chains {
				if meta.Type != merkle.ChainTypeTransaction || strings.HasPrefix(meta.Name, "synthetic-sequence(") {
					continue
				}
				c, err := batch.Account(u).ChainByName(meta.Name)
				require.NoError(t, err)
				head, err := c.Head().Get()
				require.NoError(t, err)
				k := strings.ToLower(u.String()) + "#" + meta.Name
				for i := int64(0); i < head.Count; i++ {
					h, err := c.Inner().Entry(i)
					if err != nil {
						out[k] = append(out[k], i)
						continue
					}
					if _, err := batch.Message2(h).Main().Get(); err != nil {
						out[k] = append(out[k], i)
					}
				}
			}
		}
	})
	for k, v := range out {
		t.Logf("%s: %d entries with no message behind them: %v", k, len(v), v)
	}
	return out
}
