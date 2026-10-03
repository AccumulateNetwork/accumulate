// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

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

// TestAPartitionThatLostAQuorumIsRevivedByACommitteeChange — R4-B (#4512;
// executor spec, "A partition that loses a quorum of its validators'
// states"). BVN0 has four validators and a signing threshold of three; two
// of them stop for good. With two left, nothing BVN0 sends the Directory can
// reach its quorum. The operators then remove the two from BVN0's set on the
// Directory -- the network definition's next version -- and BVN0 is two of
// two: its anchors validate, its synthetic transactions are delivered again,
// and the network runs on.
func TestAPartitionThatLostAQuorumIsRevivedByACommitteeChange(t *testing.T) {
	// The mechanism by which a halted partition adopts the committee the
	// Directory changed is a decision pending with Paul (PLAN R4-B,
	// "How a halted partition learns its new committee"): with two of four
	// validators gone BVN0's consensus has no quorum, executes nothing, and
	// so never executes the Directory anchor that carries the change -- its
	// committee is updated only by its own executed blocks (simulator
	// consensus applyValUp; the daemon's WillChangeGlobals). Today the first
	// submission after the loss fails "no response". Unskip with the fix.
	t.Skip("R4-B: pending the decision on how a halted partition adopts a committee change (#4512)")
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	carol := url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 4),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	sim.SetRoute(carol, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeIdentity(t, sim.DatabaseFor(carol), carol, acctesting.GenerateKey(carol)[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	sendTo := func(to *url.URL) *TransactionStatus {
		ts++
		return sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(to, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 3; i++ {
		st := sendTo(carol)
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// Two of BVN0's four validators are lost.
	bvn0 := sim.S.Partition("BVN0")
	lost := []int{2, 3}
	for _, i := range lost {
		bvn0.StopNode(i)
	}

	// Below its quorum, a cross-partition transfer from BVN0 is not
	// delivered: its anchors never reach the Directory's threshold.
	stuck := sendTo(carol)
	sim.StepN(60)
	require.False(t, deliveredTo(t, sim, stuck), "precondition: BVN0 below its quorum still delivered a synthetic transaction")

	// The operators remove the lost validators from BVN0's set.
	current := loadDirectoryGlobals(t, sim)
	for _, i := range lost {
		key := sim.SignWithNode("BVN0", i).(interface{ Key() []byte }).Key()[32:]
		current.Network.AddValidator(key, "BVN0", false)
	}
	current.Network.Version++
	b := build.Transaction().For(DnUrl(), Network).
		Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
		SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))
	for i := 1; i < sim.S.Partition(Directory).NodeCount(); i++ {
		b = b.SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(uint64(i + 1)).Signer(sim.SignWithNode(Directory, i))
	}
	st := sim.BuildAndSubmitTxnSuccessfully(b)
	sim.StepUntil(Txn(st.TxID).Completes())

	// BVN0 is two of two: what was stuck is delivered, and new transfers are.
	sim.StepUntil(Txn(stuck.TxID).Produced().Succeeds())
	next := sendTo(carol)
	sim.StepUntil(Txn(next.TxID).Succeeds(), Txn(next.TxID).Produced().Succeeds())
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
