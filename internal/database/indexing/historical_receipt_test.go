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

// TestHistoricalStateProof_VerifiesOffline is Phase 5's gate. It builds the full
// receipt for an account's state at a past block and requires that it validates
// with nothing but the receipt itself — no network, no database, no node.
func TestHistoricalStateProof_VerifiesOffline(t *testing.T) {
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

	// Keep the chain moving so the block alice was created in becomes history
	for i := 0; i < 12; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 2)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	// What alice's BPT entry held at the end of each block, so the receipt's
	// start can be checked against the right block rather than merely against
	// something plausible
	valueAt := map[uint64][32]byte{}
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(partition.Ledger()).Main().GetAs(&ledger))
		h, err := batch.Account(alice).Hash()
		require.NoError(t, err)
		valueAt[ledger.Index] = h
	})

	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		retained, err := indexing.RetainedBlockRange(partition, batch)
		require.NoError(t, err)
		require.False(t, retained.IsEmpty(), "retention did not take effect")
		t.Logf("retained range %v", retained)

		account := batch.Account(alice)
		first, ok, err := indexing.AccountFirstIndexedBlock(account)
		require.NoError(t, err)
		require.True(t, ok)

		current, err := batch.BPT().GetRootHash()
		require.NoError(t, err)

		checked := 0
		for h := first; h <= retained.Latest; h++ {
			proof, err := indexing.HistoricalAccountStateProof(partition, batch, account, h)
			if err != nil {
				// Heights outside the retained range are refused, which is the
				// correct answer and not a failure of this test
				require.Equalf(t, errors.IncompleteChain, errors.Code(err), "height %d: %v", h, err)
				continue
			}

			// The claim: this validates with nothing but itself
			require.Truef(t, proof.Receipt.Validate(nil),
				"the receipt for height %d does not validate offline", h)
			require.Equalf(t, current[:], proof.Receipt.Anchor,
				"height %d does not terminate at the current BPT root", h)
			require.LessOrEqualf(t, proof.Block, h, "height %d resolved forward", h)

			// The start must be alice's state hash at the block the proof names
			if want, ok := valueAt[proof.Block]; ok {
				require.Equalf(t, want[:], proof.Receipt.Start,
					"the proof for height %d starts at the wrong account state", h)
			}

			// And it must not claim to be signed
			require.Falsef(t, proof.AnchorBound,
				"height %d reported an unanchored root as anchor-bound", h)
			require.Equal(t, "BVN0", proof.Partition)
			checked++
		}
		require.Greaterf(t, checked, 3, "only %d heights produced a proof", checked)
		t.Logf("verified %d offline proofs, terminating at %x", checked, current[:6])
	})
}

// TestRetainedRange_IsPredictive is what makes advertising the range worth
// anything: a client that reads it must be able to tell which heights the node
// will answer, without asking.
//
// Every height inside the range must produce a proof, and every height below it
// must be refused. A range that over-promises is worse than no range at all,
// because a client would plan around it.
func TestRetainedRange_IsPredictive(t *testing.T) {
	liteKey := acctesting.GenerateKey(t.Name(), "lite")
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

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}

	for i := 0; i < 12; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 1)).PrivateKey(liteKey))
		sim.StepN(3)
	}

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		advertised, err := indexing.RetainedBlockRange(partition, batch)
		require.NoError(t, err)
		require.False(t, advertised.IsEmpty())
		t.Logf("advertised retained range %v", advertised)

		account := batch.Account(lite.RootIdentity())
		first, ok, err := indexing.AccountFirstIndexedBlock(account)
		require.NoError(t, err)
		require.True(t, ok)

		// The range says what the NODE retains, not what any one account can be
		// proven for. An account younger than the range is still refused below
		// its own first block, and correctly so — the two refusals are
		// different claims and the test must not conflate them.
		from := advertised.Earliest
		if first > from {
			from = first
		}
		require.Less(t, from, advertised.Latest, "no overlap; the test would prove nothing")

		inside, below := 0, 0
		for h := uint64(1); h <= advertised.Latest+3; h++ {
			_, err := indexing.HistoricalAccountStateProof(partition, batch, account, h)
			switch {
			case h >= from && h <= advertised.Latest:
				require.NoErrorf(t, err, "height %d is advertised and the account existed, but it was refused", h)
				inside++
			case h < advertised.Earliest:
				require.Errorf(t, err, "height %d is below the advertised range but was answered", h)
				below++
			}
		}
		require.Greater(t, inside, 5, "only %d heights inside the range were checked", inside)
		require.Greater(t, below, 0, "no heights below the range were checked")
		t.Logf("%d heights inside the range all answered; %d below all refused", inside, below)
	})
}
