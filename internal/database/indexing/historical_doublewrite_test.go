// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing_test

import (
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Review of !1215, finding 1.
//
// retainStateReceipt keeps the FIRST receipt written in a block:
//
//	if n := len(blocks); n > 0 && blocks[n-1] >= height { return nil }
//
// putBpt has two call sites. Batch.UpdateBPT runs it for every dirty account at
// the end of a batch, and transactionStatus.Put runs it on the principal the
// moment a transaction's status is written as Pending. A key page that is both
// the principal of a pending transaction and the signer that paid for it is
// therefore written twice in one block, with the credit balance changing in
// between — so the retained receipt may reach an entry that is no longer the
// block's final one.
//
// Measured on Kermit, ordinary traffic writes most accounts twice per block but
// always with the same root, so the guard is harmless there. This exercises the
// pending path, which Kermit never produced.
func TestHistoricalStateProof_PendingTransactionInTheSameBlock(t *testing.T) {
	liteKey := acctesting.GenerateKey("dw-lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey("dw-alice")
	page := alice.JoinPath("book", "1")

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(10_000),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), page, 1e9)
	sim.StepN(5)

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}

	// A second key, then require both, so the next transaction cannot execute
	// on one signature and must go pending.
	secondKey := acctesting.GenerateKey("dw-alice-2")
	secondHash := sha256.Sum256(secondKey[32:])
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(page).
			Body(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{Entry: KeySpecParams{KeyHash: secondHash[:]}},
				&SetThresholdKeyPageOperation{Threshold: 2},
			}}).
			SignWith(page).Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(3)

	// The page is now both the principal of a transaction that must go pending
	// and the signer that pays for it, so both putBpt call sites fire on it in
	// the same block with the credit balance changing in between.
	pending := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(page).
			Body(&UpdateKeyPage{Operation: []KeyPageOperation{&SetThresholdKeyPageOperation{Threshold: 1}}}).
			SignWith(page).Version(2).Timestamp(2).PrivateKey(aliceKey))
	sim.StepUntil(Txn(pending.TxID).IsPending())

	// Which block carried the pending status. If the page has no retained
	// receipt for it, this test proves nothing, so it is asserted rather than
	// assumed.
	var pendingBlock uint64
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		blocks, err := batch.Account(page).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		if len(blocks) == 0 {
			// Under -tags debug the observer collapses the state components into
			// one hash, so no state receipt is retained and there is no double
			// write to observe. The scenario this test builds cannot occur.
			t.Skip("the debug observer does not expose the state components")
		}
		pendingBlock = blocks[len(blocks)-1]
	})
	sim.StepN(5)

	// Every block the page retained a receipt for must still produce a proof.
	// If a later write in a block changed the entry after the receipt was
	// retained, the read path rejects the mismatch and this fails.
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		account := batch.Account(page)
		blocks, err := account.RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		if len(blocks) == 0 {
			t.Skip("nothing retained for the page; retention or the observer is not exposing components")
		}
		t.Logf("page retained receipts for %d blocks: %v (pending status landed at %d)",
			len(blocks), blocks, pendingBlock)
		require.Contains(t, blocks, pendingBlock,
			"the block carrying the pending status must have a retained receipt, or this proves nothing")

		// The page really was written twice in that block: once by
		// transactionStatus.Put for the pending status and once by UpdateBPT.
		// What matters is whether the retained receipt still reaches the
		// block's final entry.
		st := sim.QueryTransaction(pending.TxID, nil)
		require.Equal(t, errors.Pending, st.Status, "the transaction must still be pending")

		retained, err := indexing.RetainedBlockRange(partition, batch)
		require.NoError(t, err)
		require.False(t, retained.IsEmpty())

		for _, b := range blocks {
			if b < retained.Earliest || b > retained.Latest {
				continue
			}
			proof, err := indexing.HistoricalAccountStateProof(partition, batch, account, b)
			require.NoErrorf(t, err, "block %d: the retained receipt no longer reaches the block's entry", b)
			require.True(t, proof.Receipt.Validate(nil), "block %d: receipt does not validate", b)
		}
	})
}

// Review of !1215, finding 2.
//
// Every other case where the node cannot support a start at the main state —
// retention off, nothing retained, the collapsed debug observer — degrades to
// StartsAtMainState false and still returns a usable proof. An anchor mismatch
// is the one case that returns errors.InternalError and no proof at all.
//
// A mismatch means exactly "I cannot support a main-state start", so it should
// degrade the same way rather than deny a caller the entry-rooted proof it
// could still have used.
func TestHistoricalStateProof_MismatchedReceiptShouldDegrade(t *testing.T) {
	liteKey := acctesting.GenerateKey("mm-lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(10_000),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(5)

	for i := 0; i < 8; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 2)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}
	var block uint64

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		blocks, err := batch.Account(lite).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		if len(blocks) == 0 {
			t.Skip("nothing retained")
		}
		block = blocks[len(blocks)-1]

		// It works before the receipt is corrupted
		proof, err := indexing.HistoricalAccountStateProof(partition, batch, batch.Account(lite), block)
		require.NoError(t, err)
		require.True(t, proof.StartsAtMainState, "precondition: the receipt was retained and used")
	})

	// Plant a receipt that does not reach the block's entry. A node could hold
	// one for any number of dull reasons; the question is what it does then.
	Update(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		r, err := batch.Account(lite).RetainedStateReceipt(block).Get()
		require.NoError(t, err)
		bad := r.Copy()
		bad.Anchor = make([]byte, 32)
		copy(bad.Anchor, r.Anchor)
		bad.Anchor[0] ^= 0xff
		require.NoError(t, batch.Account(lite).RetainedStateReceipt(block).Put(bad))
	})

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		proof, err := indexing.HistoricalAccountStateProof(partition, batch, batch.Account(lite), block)
		require.NoError(t, err,
			"a retained receipt that does not reach the entry should degrade to StartsAtMainState=false, not fail the proof")
		require.NotNil(t, proof)
		require.False(t, proof.StartsAtMainState,
			"the node cannot support a main-state start, and must say so rather than claim it")
		require.True(t, proof.Receipt.Validate(nil), "the entry-rooted proof must still be usable")
	})
}
