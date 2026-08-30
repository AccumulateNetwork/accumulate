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

		// Every height in range resolves, forward only, onto an indexed block
		for height := indexed.Earliest; height <= indexed.Latest; height++ {
			entry, err := indexing.ResolveBlockAtOrAfter(partition, batch, height)
			require.NoErrorf(t, err, "height %d", height)
			require.GreaterOrEqualf(t, entry.BlockIndex, height,
				"resolution moved backward for height %d", height)
			require.Truef(t, isIndexed[entry.BlockIndex],
				"height %d resolved to unindexed block %d", height, entry.BlockIndex)
		}

		// Below the horizon and past the tip are both refused, distinguishably
		// from each other by message and from retention by status
		if indexed.Earliest > 1 {
			_, err = indexing.ResolveBlockAtOrAfter(partition, batch, indexed.Earliest-1)
			require.Error(t, err)
			require.Equal(t, errors.NotFound, errors.Code(err))
		}
		_, err = indexing.ResolveBlockAtOrAfter(partition, batch, indexed.Latest+1)
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
		_, err = indexing.ResolveHistoricalAccountState(partition, batch, account, first-1, 1000)
		require.Error(t, err)
		require.Equal(t, errors.NotFound, errors.Code(err))
		require.Contains(t, err.Error(), "did not exist at block")

		// With retention at its default of zero, every height in range is
		// refused with IncompleteChain. No height yields a receipt against the
		// current root.
		for height := first; height <= indexed.Latest; height++ {
			_, err := indexing.ResolveHistoricalAccountState(partition, batch, account, height, 0)
			require.Errorf(t, err, "height %d was answered", height)
			require.Equalf(t, errors.IncompleteChain, errors.Code(err), "height %d", height)
		}
	})
}
