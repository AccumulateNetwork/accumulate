// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAIP58_ProveAKeyPageAtItsOwnVersion is the acceptance test for AIP-58.
//
// Accumulate refuses a signature made against any key page version other than
// the one in force when the transaction executed. A verifier re-checking that
// later needs the page as it stood AT THE EXECUTION BLOCK — not as it stands
// now, by which time the page may have moved to a version that never authorised
// anything. Comparing against the current state produces a governance rejection
// of a transaction the network executed: a confident, checkable, wrong answer.
//
// So: create a page, execute under version 1, move the page to version 2, and
// then prove the page at the block it was version 1.
//
// The test is built so that it FAILS if the historical path silently degrades to
// the current root — which is exactly what this branch changes. It asserts the
// historical receipt starts at the page's hash as it was, and that this differs
// from the page's hash now.
func TestAIP58_ProveAKeyPageAtItsOwnVersion(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), "alice")
	keyHash := sha256.Sum256(aliceKey[32:])
	page := alice.JoinPath("book", "1")

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(10_000),
	)

	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(5)

	// A key page lives on a BVN, like every account anyone wants to prove
	partition, err := sim.Router().RouteAccount(alice)
	require.NoError(t, err)
	require.NotEqual(t, Directory, partition, "the page must be on a BVN")

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			Body(&CreateIdentity{Url: alice, KeyHash: keyHash[:], KeyBookUrl: alice.JoinPath("book")}).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	CreditCredits(t, sim.DatabaseFor(alice), page, 1e9)
	sim.StepN(10)

	// Execute something signed by the page AT VERSION 1
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			Body(&CreateTokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()}).
			SignWith(page).Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(3)

	// Capture the page as it was: its version, and the hash its BPT entry held.
	// This is the state a verifier must be able to prove later.
	var execBlock uint64
	var wasVersion uint64
	var wasHash [32]byte
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl(partition).JoinPath(Ledger)).Main().GetAs(&ledger))
		execBlock = ledger.Index

		var kp *KeyPage
		require.NoError(t, batch.Account(page).Main().GetAs(&kp))
		wasVersion = kp.Version

		wasHash, err = batch.Account(page).Hash()
		require.NoError(t, err)
	})
	require.Equal(t, uint64(1), wasVersion, "the page should still be at version 1")
	t.Logf("page was version %d at block %d, hash %x", wasVersion, execBlock, wasHash[:8])

	// Move the page to version 2
	other := sha256.Sum256([]byte("a second key"))
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(page).
			Body(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{Entry: KeySpecParams{KeyHash: other[:]}},
			}}).
			SignWith(page).Version(1).Timestamp(2).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Keep the chain moving so the execution block is genuinely history
	for i := 0; i < 8; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 10)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	querier := api.Querier2{Querier: dut.NewQuerier(dut.QuerierParams{
		Logger:    acctesting.NewTestLogger(t),
		Database:  sim.Database(partition),
		Partition: partition,
	})}
	query := func(forHeight uint64) (*api.AccountRecord, error) {
		q := new(api.DefaultQuery)
		q.IncludeReceipt = &api.ReceiptOptions{ForAny: true, ForHeight: forHeight}
		return querier.QueryAccount(context.Background(), page, q)
	}

	// The page is now version 2, and its hash has moved
	now, err := query(0)
	require.NoError(t, err)
	nowPage, ok := now.Account.(*KeyPage)
	require.True(t, ok)
	require.Equal(t, uint64(2), nowPage.Version, "the page should have moved to version 2")

	var nowHash [32]byte
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		nowHash, err = batch.Account(page).Hash()
		require.NoError(t, err)
	})
	require.NotEqual(t, wasHash, nowHash, "the page's hash did not change; the test would prove nothing")

	// THE CLAIM: ask about the execution block and get the page as it was
	past, err := query(execBlock)
	require.NoError(t, err, "the historical query was refused")
	require.NotNil(t, past.Receipt)

	require.Equal(t, wasHash[:], past.Receipt.Start,
		"the historical receipt does not start at the page as it was at the execution block")
	require.NotEqual(t, nowHash[:], past.Receipt.Start,
		"the historical receipt starts at the page as it is NOW — the past was answered with the present")
	require.LessOrEqual(t, past.Receipt.ForHeight, execBlock)
	require.NotZero(t, past.Receipt.ForHeight)

	// And it verifies with nothing but itself
	require.True(t, past.Receipt.Validate(nil), "the receipt does not verify offline")

	t.Logf("proved the page at block %d (resolved %d): start %x, anchor %x",
		execBlock, past.Receipt.ForHeight, past.Receipt.Start[:8], past.Receipt.Anchor[:8])
}

// TestAIP58_RefusesWhatItCannotProve is the other half of the acceptance
// criteria, and the more important one operationally: a node that does not
// retain history must refuse, not approximate.
//
// It is the same scenario with retention off. Every historical question is
// declined and the current-state query still works.
func TestAIP58_RefusesWhatItCannotProve(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(15)

	partition, err := sim.Router().RouteAccount(lite)
	require.NoError(t, err)

	querier := api.Querier2{Querier: dut.NewQuerier(dut.QuerierParams{
		Logger:    acctesting.NewTestLogger(t),
		Database:  sim.Database(partition),
		Partition: partition,
	})}
	query := func(forHeight uint64) (*api.AccountRecord, error) {
		q := new(api.DefaultQuery)
		q.IncludeReceipt = &api.ReceiptOptions{ForAny: true, ForHeight: forHeight}
		return querier.QueryAccount(context.Background(), lite.RootIdentity(), q)
	}

	now, err := query(0)
	require.NoError(t, err)
	require.NotNil(t, now.Receipt, "the current-state path must keep working")

	for _, h := range []uint64{1, 3, 7, 12} {
		r, err := query(h)
		require.Errorf(t, err, "block %d was answered by a node that retains nothing", h)
		require.Nil(t, r)
	}
}
