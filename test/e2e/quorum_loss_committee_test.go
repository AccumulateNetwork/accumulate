// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAPartitionThatLostAQuorumHaltsAndResumesWhenItsValidatorsReturn —
// #4512 (executor spec, "A partition that loses a quorum of its validators'
// states"). BVN0 has four validators and a quorum of three; two of them stop.
// BVN0 cannot complete a block, so it does not change state: its height
// stands. The other partitions run on -- nothing in theirs waits on BVN0.
// Then the operators restore the two with their stores and keys: they join
// from the two that stood, BVN0's committee is whole again, and it resumes,
// a cross-partition transfer from it delivered.
func TestAPartitionThatLostAQuorumHaltsAndResumesWhenItsValidatorsReturn(t *testing.T) {
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
	send := func() *TransactionStatus {
		ts++
		return sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(carol, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 3; i++ {
		st := send()
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	bvn0, bvn1, dn := sim.S.Partition("BVN0"), sim.S.Partition("BVN1"), sim.S.Partition(Directory)
	height := func(p *simulator.Partition, name string) uint64 {
		return partitionBlock(t, p.NodeDatabase(0), PartitionUrl(name))
	}

	// Two of BVN0's four are lost.
	lost := []int{2, 3}
	for _, i := range lost {
		bvn0.StopNode(i)
	}
	h0, h1, hd := height(bvn0, "BVN0"), height(bvn1, "BVN1"), height(dn, Directory)
	sim.StepN(30)
	require.Equal(t, h0, height(bvn0, "BVN0"), "BVN0 changed state with two of its four validators")
	require.Greater(t, height(bvn1, "BVN1"), h1, "BVN1 stopped with BVN0: its blocks wait on another partition")
	require.Greater(t, height(dn, Directory), hd, "the Directory stopped with BVN0: its blocks wait on another partition")

	// The operators restore the two, stores and keys as they stood.
	var back []restartTarget
	for _, i := range lost {
		bvn0.RestoreNode(i)
		back = append(back, restartTarget{"BVN0", bvn0, i})
	}
	joinRestarted(t, sim, back, 60*time.Second)

	// BVN0 resumes, and what it sends is delivered.
	st := send()
	sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	require.Greater(t, height(bvn0, "BVN0"), h0, "BVN0 did not resume once its validators returned")
}

// deliveredTo reports whether every synthetic transaction st produced has
// executed at its destination.
func deliveredTo(t *testing.T, sim *Sim, st *TransactionStatus) bool {
	t.Helper()
	r := sim.QueryTransaction(st.TxID, nil)
	if r.Produced == nil || len(r.Produced.Records) == 0 {
		return false
	}
	for _, p := range r.Produced.Records {
		pr := sim.QueryTransaction(p.Value, nil)
		if pr.Status != errors.Delivered {
			return false
		}
	}
	return true
}
