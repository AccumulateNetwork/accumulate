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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAJoinExecutesWithTheGlobalsOfTheStateItIsProvenAt is #4442's follow-up
// from run 20260928T051402Z, where every Directory follower missed on the
// first block after its proof. The executor loads its globals -- the network
// definition, oracle, globals, routing and executor versions -- from the store
// when it starts, and after that only when a block it executes writes them. A
// joining node pulls those accounts; the executor went on with what it had
// loaded before the pull. Here the network definition changes while the node
// is down, and after the join the node's executor holds the definition of the
// state it was proven at.
func TestAJoinExecutesWithTheGlobalsOfTheStateItIsProvenAt(t *testing.T) {
	f := newProvenFixtureWith(t, "Directory", func(sim *Sim) {
		current := loadDirectoryGlobals(t, sim)
		newKey := acctesting.GenerateKey(t.Name(), "new-validator")
		current.Network.AddValidator(newKey[32:], "BVN0", false)
		current.Network.Version++
		b := build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))
		for i := 1; i < sim.S.Partition(Directory).NodeCount(); i++ {
			b = b.SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(uint64(i + 1)).Signer(sim.SignWithNode(Directory, i))
		}
		st := sim.BuildAndSubmitTxnSuccessfully(b)
		sim.StepUntil(Txn(st.TxID).Completes())
	})
	joinerBefore := f.p.NodeConductor(provenJoiner).Globals.Load().Network.Version
	want := loadDirectoryGlobals(t, f.sim).Network.Version
	require.Less(t, joinerBefore, want, "precondition: the joining node's executor holds an older network definition")

	s, err := f.join(t, 150, nil)
	require.NoError(t, err)
	require.NotEmpty(t, s.handoffs)
	require.Equal(t, want, f.p.NodeConductor(provenJoiner).Globals.Load().Network.Version,
		"the joined node executes with the network definition it started with, not the proven state's")
	require.Zero(t, s.diverged)
	f.requireInAgreement(t)
}

// TestAJoinDoesNotBurnAgainWhatItsPeersSkipped is #4442's follow-up from run
// 20260928T051402Z, where every Directory follower's first block after its
// proof left acc://ACME's issued amount 3,008,000 below its peers'. A burn
// the Directory makes for its own credit purchases is queued as a local
// delivery (a SyntheticBurnTokens to acc://ACME carrying only its amount) and
// drained at the next block. Two burns of one amount are one transaction by
// hash, and the peers, holding the first one's status, skip the second. That
// status is not in the BPT: the joined node held none, and burned again.
//
// A Directory account buys the same credits every block, so every block the
// follower can be proven at queues a burn its peers will skip.
func TestAJoinDoesNotBurnAgainWhatItsPeersSkipped(t *testing.T) {
	dave := url.MustParse("dave")
	daveKey := acctesting.GenerateKey(dave)
	var ts uint64
	f := newProvenFixtureWith(t, "Directory", func(sim *Sim) {
		sim.SetRoute(dave, Directory)
		MakeIdentity(t, sim.DatabaseFor(dave), dave, daveKey[32:])
		CreditCredits(t, sim.DatabaseFor(dave), dave.JoinPath("book", "1"), 1e12)
		MakeAccount(t, sim.DatabaseFor(dave), &TokenAccount{Url: dave.JoinPath("tokens"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(dave), dave.JoinPath("tokens"), big.NewInt(1e12))
	}, simulator.BPTHistoryDepth(4096))
	traffic := f.traffic
	f.traffic = func() {
		ts++
		f.sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(dave, "tokens").
				AddCredits().Spend(1).To(dave.JoinPath("book", "1")).WithOracle(InitialAcmeOracle).
				SignWith(dave, "book", "1").Version(1).Timestamp(ts).PrivateKey(daveKey))
		traffic()
	}
	for i := 0; i < 5; i++ {
		f.traffic()
	}

	s, err := f.join(t, 150, nil)
	require.NoError(t, err)
	require.NotEmpty(t, s.handoffs)
	require.Zero(t, s.diverged, "the follower executed a block other than its peers' %d times after handing off (handoffs at %v)",
		s.diverged, handoffBlocks(s))
	f.requireInAgreement(t)
}
