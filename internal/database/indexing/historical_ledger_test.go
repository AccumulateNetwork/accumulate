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

// TestHistoricalResolution_RealLedger exercises the resolver against a ledger
// built by executing real transactions, rather than against a synthetic index
// chain. The synthetic tests fix the semantics; this one establishes that the
// semantics match what a partition actually produces.
func TestHistoricalResolution_RealLedger(t *testing.T) {
	liteKey := acctesting.GenerateKey("lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey("alice")
	keyHash := sha256.Sum256(aliceKey[32:])

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(10)

	// Create alice with a real transaction, so its chains are written and
	// indexed by the executor rather than by a test helper
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			Body(&CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			SignWith(lite.RootIdentity()).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(20)

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}

	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		indexed, err := indexing.IndexedBlockRange(partition, batch)
		require.NoError(t, err)
		require.NotZero(t, indexed.Earliest)
		require.Greater(t, indexed.Latest, indexed.Earliest)

		// Collect the blocks the ledger actually indexed
		rootIndex, err := batch.Account(partition.Ledger()).RootChain().Index().Get()
		require.NoError(t, err)
		isIndexed := map[uint64]bool{}
		for i := int64(0); i < rootIndex.Height(); i++ {
			entry := new(IndexEntry)
			require.NoError(t, rootIndex.EntryAs(i, entry))
			isIndexed[entry.BlockIndex] = true
		}

		// A partition does not index every block — it indexes the blocks that
		// produced a root chain entry. If that were not so, at-or-after
		// resolution would be dead code, so assert the gaps exist.
		require.Less(t, uint64(rootIndex.Height()), indexed.Latest-indexed.Earliest+1,
			"expected the root index chain to be sparse")

		// Every height in range resolves, backward only, onto an indexed block
		for height := indexed.Earliest; height <= indexed.Latest; height++ {
			_, entry, err := indexing.ResolveBlockAtOrBefore(partition, batch, height)
			require.NoErrorf(t, err, "height %d", height)
			require.LessOrEqualf(t, entry.BlockIndex, height,
				"resolution moved forward for height %d", height)
			require.Truef(t, isIndexed[entry.BlockIndex],
				"height %d resolved to unindexed block %d", height, entry.BlockIndex)
		}

		// Below the horizon and past the tip are both refused, distinguishably
		// from each other by message and from retention by status
		if indexed.Earliest > 1 {
			_, _, err = indexing.ResolveBlockAtOrBefore(partition, batch, indexed.Earliest-1)
			require.Error(t, err)
			require.Equal(t, errors.NotFound, errors.Code(err))
		}
		_, _, err = indexing.ResolveBlockAtOrBefore(partition, batch, indexed.Latest+1)
		require.Error(t, err)
		require.Equal(t, errors.NotFound, errors.Code(err))

		// alice was created by a transaction, so it has an indexed main chain
		account := batch.Account(alice)
		first, ok, err := indexing.AccountFirstIndexedBlock(account)
		require.NoError(t, err)
		require.True(t, ok, "alice should have an indexed main chain")
		require.True(t, indexed.Contains(first))

		// Asking about a block before alice existed is refused as NotFound even
		// with retention enabled, because the account was not there to prove
		_, err = indexing.ResolveHistoricalAccountState(partition, batch, account, first-1)
		require.Error(t, err)
		require.Equal(t, errors.NotFound, errors.Code(err))
		require.Contains(t, err.Error(), "did not exist at block")

		// With retention at its default of zero, every height in range is
		// refused with IncompleteChain. No height yields a receipt against the
		// current root.
		for height := first; height <= indexed.Latest; height++ {
			_, err := indexing.ResolveHistoricalAccountState(partition, batch, account, height)
			require.Errorf(t, err, "height %d was answered", height)
			require.Equalf(t, errors.IncompleteChain, errors.Code(err), "height %d", height)
		}
	})
}

// TestBPTRootAt_RealLedger locks the offset between the ledger's BptChain and
// the root index chain by capturing the real BPT root after every block and
// requiring BPTRootAt to return exactly that root for that block.
//
// The offset is not cosmetic. BptChain records the *previous* block's state
// hash, so an off-by-one here would serve the neighbouring block's state — with
// a valid receipt and a truthful height. That is the failure this whole
// mechanism exists to prevent, so it is asserted against measured roots rather
// than reasoned about.
func TestBPTRootAt_RealLedger(t *testing.T) {
	liteKey := acctesting.GenerateKey("lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}

	// Capture the real BPT root at the end of every state-changing block
	rootAtBlock := map[uint64][32]byte{}
	capture := func() {
		View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
			var ledger *SystemLedger
			require.NoError(t, batch.Account(partition.Ledger()).Main().GetAs(&ledger))
			root, err := batch.BPT().GetRootHash()
			require.NoError(t, err)
			rootAtBlock[ledger.Index] = root
		})
	}
	// Step once before capturing anything. MakeLiteTokenAccount and the credit
	// helpers write straight to the database, changing the BPT outside block
	// execution, so the root observed before the next block commits does not
	// correspond to any BptChain entry.
	sim.StepN(2)
	var firstCaptured uint64
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(partition.Ledger()).Main().GetAs(&ledger))
		firstCaptured = ledger.Index
	})
	// Real transactions, so blocks actually change state — an idle simulator
	// produces too few indexed blocks for this to prove anything
	for i := 0; i < 12; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(lite).
				AddCredits().Spend(1).To(lite.RootIdentity()).WithOracle(InitialAcmeOracle).
				SignWith(lite.RootIdentity()).Version(1).Timestamp(uint64(i + 1)).PrivateKey(liteKey))
		for j := 0; j < 3; j++ {
			sim.StepN(1)
			capture()
		}
	}
	delete(rootAtBlock, firstCaptured)

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		indexed, err := indexing.IndexedBlockRange(partition, batch)
		require.NoError(t, err)

		checked := 0
		for block, want := range rootAtBlock {
			if block <= firstCaptured || block > indexed.Latest {
				continue
			}
			got, at, err := indexing.BPTRootAt(partition, batch, block)
			if err != nil {
				// The most recent block's root is not on the chain yet — it is
				// written when the next state-changing block commits. That is a
				// documented refusal, not a failure.
				require.Equalf(t, errors.IncompleteChain, errors.Code(err), "block %d", block)
				continue
			}
			require.Equalf(t, block, at, "BPTRootAt(%d) reported block %d", block, at)
			require.Equalf(t, want, got, "wrong root for block %d", block)
			checked++
		}
		require.Greaterf(t, checked, 5, "only %d blocks checked; the test proved little", checked)

		// A height in a gap returns the previous block's root, unchanged
		for block := range rootAtBlock {
			if block <= firstCaptured+1 || block > indexed.Latest {
				continue
			}
			got, at, err := indexing.BPTRootAt(partition, batch, block-1)
			if err != nil {
				continue
			}
			require.LessOrEqualf(t, at, block-1, "resolution moved forward from %d", block-1)
			if want, ok := rootAtBlock[at]; ok {
				require.Equalf(t, want, got, "gap resolution returned the wrong root for %d", block-1)
			}
		}
	})
}

// TestRetention_DefaultIsOff runs a real simulator, with the executor wired to
// whatever BPTHistoryDepth defaults to, and requires that the node retains
// nothing and advertises an empty range.
//
// This is the end of the config path, asserted rather than assumed: a mistake in
// the default would silently start writing history on every node that upgrades,
// which is exactly the surprise the depth-0 default exists to prevent.
func TestRetention_DefaultIsOff(t *testing.T) {
	liteKey := acctesting.GenerateKey("lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e12))
	sim.StepN(25)

	partition := config.NetworkUrl{URL: PartitionUrl("BVN0")}
	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		_, ok, err := batch.BPT().EarliestRetained()
		require.NoError(t, err)
		require.False(t, ok, "the default configuration retained BPT history")

		r, err := indexing.RetainedBlockRange(partition, batch)
		require.NoError(t, err)
		require.True(t, r.IsEmpty(), "a node retaining nothing advertised %v", r)
	})
}
