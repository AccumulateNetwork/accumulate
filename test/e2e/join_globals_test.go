// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
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
