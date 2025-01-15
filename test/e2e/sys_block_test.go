// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestBptChain(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Trigger a block
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(100).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Get the BVN's BPT hash
	var bptHash [32]byte
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		var err error
		bptHash, err = batch.GetBptRootHash()
		require.NoError(t, err)
	})

	// Trigger another block
	sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(100).
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey))

	// Step once
	sim.Step()

	// Verify the BPT chain
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		account := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger))
		chains, err := account.Chains().Get()
		require.NoError(t, err)

		var found bool
		for _, c := range chains {
			if c.Name == "bpt" {
				found = true
				break
			}
		}
		require.True(t, found)

		chain := account.BptChain()
		head, err := chain.Head().Get()
		require.NoError(t, err)
		require.NotZero(t, head.Count)

		entry, err := chain.Entry(head.Count - 1)
		require.NoError(t, err)
		require.Equal(t, bptHash[:], entry)
	})
}

func TestQuiescence(t *testing.T) {
	// Tests https://gitlab.com/accumulatenetwork/accumulate/-/issues/3453?work_item_iid=3520

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Give the network time to stabilize
	sim.StepN(100)

	// Verify that the network is (and stays) quiescent
	before := GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger))
	sim.StepN(100)
	after := GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger))
	require.Equal(t, int(before.Index), int(after.Index))
}
