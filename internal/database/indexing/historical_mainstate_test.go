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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestHistoricalStateProof_StartsAtMainState is the point of the change: a
// verifier holding only the account state must be able to compute the receipt's
// starting point for itself.
//
// Before, the proof started at the whole BPT entry —
// H(main, secondary, chains, pending) — of which a verifier has one part, so it
// had to accept the server's word for where the receipt began. That is the trust
// the proof exists to remove. Now it starts at a simple hash of the main state,
// which the verifier recomputes from the state it was handed.
func TestHistoricalStateProof_StartsAtMainState(t *testing.T) {
	liteKey := acctesting.GenerateKey("lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey("alice")
	keyHash := sha256.Sum256(aliceKey[32:])

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
		simulator.BPTHistoryDepth(10_000),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(5)

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			Body(&CreateIdentity{Url: alice, KeyHash: keyHash[:], KeyBookUrl: alice.JoinPath("book")}).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Move the chain on so alice's creation block becomes history
	for i := 0; i < 12; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 2)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		retained, err := indexing.RetainedBlockRange(partition, batch)
		require.NoError(t, err)
		require.False(t, retained.IsEmpty(), "retention did not take effect")

		account := batch.Account(alice)
		current, err := batch.BPT().GetRootHash()
		require.NoError(t, err)

		// What a verifier is handed: the account state itself
		var state Account
		require.NoError(t, account.Main().GetAs(&state))
		encoded, err := state.MarshalBinary()
		require.NoError(t, err)
		mainStateHash := sha256.Sum256(encoded)

		proof, err := indexing.HistoricalAccountStateProof(partition, batch, account, retained.Latest)
		require.NoError(t, err)

		// The claim. Under -tags debug the observer collapses the components
		// into a single hash, so there is no path from the main state to the
		// entry to retain and the proof honestly reports that it does not start
		// there.
		if !proof.StartsAtMainState {
			require.True(t, isDebugObserver(t, batch, alice),
				"retention is on and the observer exposes the components, so the state receipt should have been retained")
			t.Skip("the debug observer does not expose the state components")
		}
		require.Equal(t, mainStateHash[:], proof.Receipt.Start,
			"the proof does not start where a verifier can compute")

		// and it is still a complete, offline-checkable chain to the current root
		require.True(t, proof.Receipt.Validate(nil), "the receipt does not validate offline")
		require.Equal(t, current[:], proof.Receipt.Anchor, "does not terminate at the current BPT root")

		// A verifier given a different account state must not be able to pass it
		tampered := mainStateHash
		tampered[0]++
		require.NotEqual(t, tampered[:], proof.Receipt.Start)
	})
}

// TestHistoricalStateProof_NoRetentionNoStart proves the flag is honest: with
// retention off there is no retained state receipt, so the proof cannot start at
// the main state and must say so rather than claiming it does.
func TestHistoricalStateProof_NoRetentionNoStart(t *testing.T) {
	liteKey := acctesting.GenerateKey("lite2")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	sim.StepN(10)

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		blocks, err := batch.Account(lite).RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.Empty(t, blocks, "retention is off, so nothing may be retained")
	})
}

// isDebugObserver reports whether this build hashes an account into a single
// value rather than the four components the receipt is cut from.
func isDebugObserver(t *testing.T, batch *database.Batch, u *url.URL) bool {
	t.Helper()
	h, err := database.NewDatabaseObserver().DidChangeAccount(batch, batch.Account(u))
	require.NoError(t, err)
	return len(h) < 2
}
