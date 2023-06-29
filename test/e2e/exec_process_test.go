// Copyright 2023 The Accumulate Authors
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
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestExecutorProcessResults verifies that execute.Executor.Process strips the
// error message and specific error code from the transaction result, so that
// differences in the error message or code do not cause consensus failures.
func TestExecutorProcessResults(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3), // The node count must be > 1
		simulator.Genesis(GenesisTime),
	)

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Settle
	sim.StepN(10)

	// Submit a transaction that will fail
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1000, 0).To("bob").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// This will be called once per node
	var i int64
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), func(a *TokenAccount) {
		// Set the balance on each node to a different value
		i++
		a.Balance = *big.NewInt(i)
	})
	require.Equal(t, 3, int(i))

	// Screwing with the account balance will cause the BPT to differ, so instruct the simulator to ignore that
	sim.S.IgnoreCommitResults = true

	// Verify the error message (which ends up coming from the first node)
	sim.StepUntil(
		Txn(st.TxID).Fails().
			WithMessage("insufficient balance: have 1, want 1000"))
}
