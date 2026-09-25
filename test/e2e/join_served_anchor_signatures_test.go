// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
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

// #4413. Run 20260924T074702Z, acc-bvn3-val3 joining BVN3 at 08:19:17:
//
//	An anchor was refused error={"code":"unauthenticated","codeID":401,
//	"message":"the anchor carries no signatures"} block=1388 module=join
//	partition=acc://bvn-BVN3.acme
//
// 22 times, for BVN3 blocks 1388-1409 -- one 64-entry page of dn.acme/anchors
// read by anchorsrc from a Directory peer. The Directory executed those
// anchors in its blocks 1362-1383 (08:11:55-08:12:17). acc-bvn1-val3 was
// restarted at 08:11:39 having executed Directory block 1350, and its
// Directory join pulled the state through block 1432 (`synced=1432`) and
// turned ACTIVE at 08:13:03 -- so it held those pool entries, and it held them
// only by pull.
//
// The pull brings a chain's entries and, since #4400, the message behind each
// entry. It does not bring what the query API reads an anchor's signatures
// from: the transaction's status (AnchorSigners, Signers) and the pool
// account's per-transaction signature history (internal/api/v3/load.go,
// loadTransactionSignaturesV1/V2). So every anchor in a joined node's pulled
// range is served as a body with no signatures, and every node that later
// reads its roots from that peer refuses them.
//
// The control is the same read against a peer that executed every block.
//
// #4416 is the fix: the signature chain is pulled with the message behind each
// entry (#4400), and the pull now writes beside each anchor signature what
// executing it wrote -- the history index, the signer, the sequenced message
// and the cause -- so the joined node serves every anchor in its pulled range
// signed, over the sequence the signatures cover.
func TestAJoinedNodeServesTheAnchorsItPulledWithTheirSignatures(t *testing.T) {

	const joiner = 1
	sim, p, r, q := joinADirectoryNodeByPull(t)
	dn := DnUrl()

	// What a joining BVN0 node does to find its roots: anchorsrc reads
	// dn.acme/anchors from a Directory peer and verifies each BVN0 anchor's
	// signatures against the sets in its own store. Here the peer is one
	// Directory node's querier over its own store -- what that node serves.
	bvn := PartitionUrl("BVN0")
	refusedBy := func(node int) map[uint64]string {
		authority, err := anchorsrc.FromStore(sim.S.Partition("BVN0").NodeDatabase(0), bvn)
		require.NoError(t, err)
		pool := DnUrl().JoinPath(AnchorPool)
		params := apiimpl.QuerierParams{Partition: Directory, Database: p.NodeDatabase(node)}
		if js := p.NodeJoinState(node); js != nil {
			// The gate the daemon puts in front of the querier (#4368): a
			// node that has not joined refuses; the joined node reads ACTIVE.
			params.NodeState = js.Machine()
		}
		served := apiimpl.NewQuerier(params)
		verified, refused, err := readPoolAnchors(context.Background(), served, pool, bvn, authority, 1)
		require.NoError(t, err)
		t.Logf("node %d: %d BVN0 anchors verified, %d refused", node, len(verified), len(refused))
		return refused
	}

	require.Empty(t, refusedBy(0), "control: a Directory node that executed every block serves every anchor with its signatures")

	refused := refusedBy(joiner)
	var blocks []uint64
	for block := range refused {
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	// Where the query API reads an anchor's signatures from, on the joined
	// node and on the control, for every pool entry: the transaction status
	// (V1: AnchorSigners, Signers) and the pool's per-transaction history (V2).
	sigSources := func(node int) (entries, withSigners, withHistory int) {
		View(t, p.NodeDatabase(node), func(batch *database.Batch) {
			pool := batch.Account(dn.JoinPath(AnchorPool))
			head, err := pool.MainChain().Head().Get()
			require.NoError(t, err)
			for i := int64(0); i < head.Count; i++ {
				h, err := pool.MainChain().Entry(i)
				require.NoError(t, err)
				entries++
				st, err := batch.Transaction(h).Status().Get()
				if err == nil && (len(st.AnchorSigners) > 0 || len(st.Signers) > 0) {
					withSigners++
				}
				hist, err := pool.Transaction(*(*[32]byte)(h)).History().Get()
				if err == nil && len(hist) > 0 {
					withHistory++
				}
			}
		})
		return
	}
	for _, node := range []int{0, joiner} {
		e, s, h := sigSources(node)
		t.Logf("node %d: dn.acme/anchors has %d entries; %d have a status naming signers, %d have a signature history", node, e, s, h)
	}

	// Every refusal, not only "no signatures": with the history and without
	// the sequence it covers, the joined node served each of these anchors
	// with signatures over a sequence it could not name, and the reader
	// counted none of them (#4416).
	require.Empty(t, refused,
		"a joined Directory node serves %d BVN0 anchors (blocks %v) that do not verify -- the ones appended by blocks it did not execute (R=%d..Q=%d)", len(blocks), blocks, r, q)
}

// joinADirectoryNodeByPull restarts Directory node 1, runs the network on
// without it, and joins it with the production pull. It returns the block the
// node stopped at (R) and the one it joined at (Q): every anchor on
// dn.acme/anchors appended in between, the node holds only by pull.
func joinADirectoryNodeByPull(t *testing.T) (*Sim, *simulator.Partition, uint64, uint64) {
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
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := uint64(1); i <= 5; i++ {
		send(i)
	}
	sim.StepN(10)

	dn := DnUrl()
	p := sim.S.Partition("Directory")
	r := partitionBlock(t, p.NodeDatabase(joiner), dn)

	// A Directory validator restarts and the network runs on without it: BVN0
	// anchors keep landing on dn.acme/anchors on its peers and not on it.
	p.RestartNode(joiner)
	require.True(t, p.Joining(joiner))
	for i := uint64(6); i <= 10; i++ {
		send(i)
	}
	sim.StepN(20)

	// The join, as the daemon runs it, with the production pull.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{cancel: cancel, step: func(round int) {
		sim.StepN(3)
		if round >= 200 {
			cancel()
		}
	}}
	stepping.State = p.NodeJoinState(joiner)
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "Directory",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "Directory", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.False(t, p.Joining(joiner))
	require.True(t, p.NodeJoinState(joiner).Machine().CanServeCurrent(),
		"precondition: the joined node reads ACTIVE, so its querier serves (#4368)")

	q := partitionBlock(t, p.NodeDatabase(joiner), dn)
	t.Logf("the Directory node stopped at R=%d and joined at Q=%d", r, q)
	require.Greater(t, q, r, "precondition: the join carried the node past blocks it did not execute")
	return sim, p, r, q
}
