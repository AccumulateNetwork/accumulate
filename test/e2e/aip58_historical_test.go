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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	testhttp "gitlab.com/accumulatenetwork/accumulate/test/util/http"
)

// TestAIP58_ProveAKeyPageAtItsOwnVersion is main's AIP-58 acceptance test,
// ported (#4361).
//
// Accumulate refuses a signature made against any key page version other than
// the one in force when the transaction executed. A verifier re-checking that
// later needs the page as it stood AT THE EXECUTION BLOCK — not as it stands
// now, by which time the page may have moved to a version that never
// authorised anything. Comparing against the current state produces a
// governance rejection of a transaction the network executed: a confident,
// checkable, wrong answer.
//
// So: create a page, execute under version 1, move the page to version 2, and
// then ask for the page at the block it was version 1.
//
// # What this asserts that main's does not
//
// Main returns the CURRENT body with the historical receipt, and its test
// compares the receipt's start against a hash it read out of the database
// beforehand. Here the response is coherent or it is a refusal, so the test
// asks the response alone: the body must BE the page at version 1, and the
// receipt must start at the hash of that body. Nothing is read from the
// database to make the assertion, and `batch.UpdateBPT()` is never called by
// hand — the state under test is the state the executor committed.
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

	// The block the page was at version 1, read the way any client reads it
	c := realClient(t, sim)
	ctx := context.Background()
	execBlock := ledgerIndexOf(t, c, partition)

	var wasPage *KeyPage
	_, err = c.QueryAccountAs(ctx, page, nil, &wasPage)
	require.NoError(t, err)
	require.Equal(t, uint64(1), wasPage.Version, "the page should still be at version 1")

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

	// The page is now version 2
	now, err := c.QueryAccount(ctx, page, &api.DefaultQuery{
		IncludeReceipt: &api.ReceiptOptions{ForAny: true},
	})
	require.NoError(t, err)
	nowPage, ok := now.Account.(*KeyPage)
	require.True(t, ok)
	require.Equal(t, uint64(2), nowPage.Version, "the page should have moved to version 2")

	// THE CLAIM: ask about the execution block and get the page as it was
	past, err := c.QueryAccount(ctx, page, &api.DefaultQuery{
		IncludeReceipt: &api.ReceiptOptions{ForHeight: execBlock},
	})
	require.NoError(t, err, "the historical query was refused")
	require.NotNil(t, past.Receipt)

	pastPage, ok := past.Account.(*KeyPage)
	require.True(t, ok)
	require.Equal(t, uint64(1), pastPage.Version,
		"the page was answered at its CURRENT version; the past was answered with the present")

	// And the receipt proves the body that came with it, from that body alone
	body, err := pastPage.MarshalBinary()
	require.NoError(t, err)
	h := sha256.Sum256(body)
	require.True(t, past.Receipt.StartsAtMainState)
	require.Equal(t, h[:], past.Receipt.Start,
		"the receipt does not start at the body it was served with")
	require.True(t, past.Receipt.Validate(nil), "the receipt does not verify offline")

	require.NotZero(t, past.Receipt.ForHeight)
	require.LessOrEqual(t, past.Receipt.ForHeight, execBlock)
	require.NotEqual(t, now.Receipt.Anchor, past.Receipt.Anchor,
		"the historical receipt ends at the current root")

	// Two independent answers about the same past block must agree on what the
	// tree was: the receipt's terminus and the page query's root.
	rec, err := c.Query(ctx, PartitionUrl(partition), &api.BptPageQuery{
		Count: 8, ForHeight: past.Receipt.ForHeight,
	})
	require.NoError(t, err)
	bptPage, ok := rec.(*api.BptPageRecord)
	require.True(t, ok)
	require.Equal(t, bptPage.BptRoot[:], past.Receipt.Anchor,
		"the receipt and the BPT page disagree about the root as of block %d", past.Receipt.ForHeight)

	t.Logf("proved the page at block %d (resolved %d): version %d, start %x, anchor %x",
		execBlock, past.Receipt.ForHeight, pastPage.Version, past.Receipt.Start[:8], past.Receipt.Anchor[:8])
}

// TestAIP58_RefusesWhatItCannotProve is main's other half, ported: a node that
// does not retain history must refuse, not approximate.
//
// The daemon-level version of this is
// TestANodeThatRetainsNoBptHistoryRefuses in cmd/accumulated/run, on a real
// node with a configured depth of zero. This one covers the same ground where
// the rest of the AIP-58 suite lives.
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

	c := realClient(t, sim)
	ctx := context.Background()

	now, err := c.QueryAccount(ctx, lite.RootIdentity(), &api.DefaultQuery{
		IncludeReceipt: &api.ReceiptOptions{ForAny: true},
	})
	require.NoError(t, err)
	require.NotNil(t, now.Receipt, "the current-state path must keep working")

	for _, h := range []uint64{1, 3, 7, 12} {
		r, err := c.QueryAccount(ctx, lite.RootIdentity(), &api.DefaultQuery{
			IncludeReceipt: &api.ReceiptOptions{ForHeight: h},
		})
		require.Errorf(t, err, "block %d was answered by a node that retains nothing", h)
		require.Nil(t, r)

		p, err := c.Query(ctx, PartitionUrl(partition), &api.BptPageQuery{Count: 4, ForHeight: h})
		require.Errorf(t, err, "a page for block %d was served by a node that retains nothing", h)
		require.Nil(t, p)
	}
}

// realClient is the JSON-RPC client over the simulator's registered services:
// the same handler a node serves, the same wire encoding, the same decoding.
// It is not api.Querier2{Querier: sim.S.Services()} — that skips the wire, so
// it cannot tell whether a new field on a record or a query survives it, and
// this change adds three.
func realClient(t *testing.T, sim *Sim) api.Querier2 {
	t.Helper()
	h, err := jsonrpc.NewHandler(jsonrpc.Querier{Querier: sim.S.Services()})
	require.NoError(t, err)
	c := jsonrpc.NewClient("http://simulator/v3")
	c.Client = *testhttp.DirectHttpClient(h)
	return api.Querier2{Querier: c}
}

// ledgerIndexOf reads the partition's current block from its ledger, over the
// same client.
func ledgerIndexOf(t *testing.T, c api.Querier2, partition string) uint64 {
	t.Helper()
	var ledger *SystemLedger
	_, err := c.QueryAccountAs(context.Background(), PartitionUrl(partition).JoinPath(Ledger), nil, &ledger)
	require.NoError(t, err)
	return ledger.Index
}
