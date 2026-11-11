// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/lastcomplete"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAPartitionThatLostItsQuorumStatesItsLastCompleteBlock — #4512. BVN0
// loses two of its four validators. Each survivor, asked what the last
// complete block of its partition is, answers from the Directory's record of
// what it executed of BVN0's anchors (a BVN's own pool never holds its
// peers' signatures): the same block N on both, N the block of the newest
// anchor the Directory delivered, the root N's anchor carries, N at or below
// the survivor's height, and the root equal to the one the node recorded for
// N (BPTRootAt). A survivor past N, with no history kept, cannot obtain the
// state as of N; one at N has it at hand.
func TestAPartitionThatLostItsQuorumStatesItsLastCompleteBlock(t *testing.T) {
	alice := url.MustParse("alice")
	carol := url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 4),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(carol, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(carol), carol, acctesting.GenerateKey(carol)[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	for i := 0; i < 3; i++ {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(carol, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	bvn0, dn := sim.S.Partition("BVN0"), sim.S.Partition(Directory)
	for _, i := range []int{2, 3} {
		bvn0.StopNode(i)
	}
	sim.StepN(30)

	// What the Directory executed of BVN0's anchors, and the anchor it
	// numbers last: the expected N and root, read from the anchor chain.
	var ledger *AnchorLedger
	require.NoError(t, dn.NodeDatabase(0).View(func(b *database.Batch) error {
		return b.Account(DnUrl().JoinPath(AnchorPool)).Main().GetAs(&ledger)
	}))

	var want *PartitionAnchor
	var wantSeq uint64
	require.NoError(t, bvn0.NodeDatabase(0).View(func(b *database.Batch) error {
		for _, s := range ledger.Sequence {
			if s.Url.Equal(PartitionUrl("BVN0")) {
				wantSeq = s.Delivered
			}
		}
		var err error
		want, err = lastcomplete.AnchorOf(b, PartitionUrl("BVN0"), wantSeq)
		return err
	}))
	require.NotZero(t, wantSeq, "the Directory delivered none of BVN0's anchors")

	var first lastcomplete.Answer
	for _, i := range []int{0, 1} {
		got := lastcomplete.Find(PartitionUrl("BVN0"), bvn0.NodeDatabase(i), ledger, nil)
		require.True(t, got.Known, "node %d: %s", i, got.Why)
		require.Equal(t, want.MinorBlockIndex, got.Block, "node %d: last complete block", i)
		require.Equal(t, want.StateTreeAnchor, got.Root, "node %d: the root the anchor carries", i)
		require.LessOrEqual(t, got.Block, got.Executed, "node %d: N is above its executed height: N=%d executed=%d", i, got.Block, got.Executed)
		// The mapping from the Directory's number to a block is checked
		// against a second source: the root the node recorded for N.
		require.NoError(t, bvn0.NodeDatabase(i).View(func(b *database.Batch) error {
			root, blk, err := indexing.BPTRootAt(config.NetworkUrl{URL: PartitionUrl("BVN0")}, b, got.Block)
			require.NoError(t, err)
			require.Equal(t, got.Block, blk)
			require.Equal(t, got.Root, root, "node %d: the anchored root is not the root recorded for N", i)
			return nil
		}))
		t.Logf("node %d: %v", i, got)
		if got.Executed == got.Block {
			require.Equal(t, lastcomplete.AtHand, got.State, "node %d executed N", i)
		} else {
			require.Equal(t, lastcomplete.NotObtainable, got.State, "node %d is past N and keeps no history", i)
		}
		if i == 0 {
			first = got
		} else {
			require.Equal(t, first.Block, got.Block, "the survivors name different blocks")
		}
	}
}

// TestALastCompleteBlockIsNeverGuessedFromTheNodesOwnState — D2 (#4512): a
// node that cannot read the Directory, or finds nothing the Directory
// delivered of its partition, says N is unknown and why.
func TestALastCompleteBlockIsNeverGuessedFromTheNodesOwnState(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(5)
	db := sim.S.Partition("BVN0").NodeDatabase(0)
	part := PartitionUrl("BVN0")

	got := lastcomplete.Find(part, db, nil, errors.New("no peer serves the Directory"))
	require.False(t, got.Known)
	require.Contains(t, got.Why, "no peer serves the Directory")
	require.NotZero(t, got.Executed)

	got = lastcomplete.Find(part, db, new(AnchorLedger), nil)
	require.False(t, got.Known)
	require.Contains(t, got.Why, "none of this partition's anchors")

	got = lastcomplete.Find(part, db, nil, nil)
	require.False(t, got.Known)
}
