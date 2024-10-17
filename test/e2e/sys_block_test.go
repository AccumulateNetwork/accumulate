// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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

func TestBlockLedger(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).With(alice),
	)

	// Do something
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 1).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Check the block ledger
	var accounts []string
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		for r := range batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).BlockLedger().Scan().All() {
			_, bl, err := r.Get()
			require.NoError(t, err)
			for _, e := range bl.Entries {
				if !e.Account.RootIdentity().Equal(PartitionUrl("BVN0")) {
					accounts = append(accounts, e.Account.WithFragment(e.Chain).String())
				}
			}
		}
	})
	require.Contains(t, accounts, "acc://alice.acme/tokens#main")
}

func TestBlockLedgerUpgrade(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).With(alice).WithVersion(ExecutorVersionV2Vandenberg),
	)

	// Do something
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 1).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Verify an old ledger account exists
	ledger := GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger))
	blu := ledger.Url.JoinPath(fmt.Sprint(ledger.Index))
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		account := batch.Account(blu)
		require.NoError(t, account.Main().GetAs(new(*BlockLedger)))
		_, err := batch.BPT().Get(account.Key())
		require.NoError(t, err)
	})

	// Upgrade
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2Jiuquan).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		VersionIs(ExecutorVersionV2Jiuquan))

	// Convert the index
	Update(t, sim.Database("BVN0"), func(batch *database.Batch) {
		bli := indexing.NewBlockLedgerIndexer(context.Background(), batch, "BVN0")
		<-bli.ScanDone()
		bli.Write(batch)
	})

	// Check the block ledger
	var accounts []string
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		for r := range batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).BlockLedger().Scan().All() {
			_, bl, err := r.Get()
			require.NoError(t, err)
			for _, e := range bl.Entries {
				if !e.Account.RootIdentity().Equal(PartitionUrl("BVN0")) {
					accounts = append(accounts, e.Account.WithFragment(e.Chain).String())
				}
			}
		}
	})
	require.Contains(t, accounts, "acc://alice.acme/tokens#main")

	// Verify the old ledger account is no longer in the BPT
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		account := batch.Account(blu)
		_, err := batch.BPT().Get(account.Key())
		require.ErrorIs(t, err, errors.NotFound)
	})
}
