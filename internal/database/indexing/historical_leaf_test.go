// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing_test

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Every leaf a node retains for a block rebuilds the entry it retained for
// that block (#4361). The accounts are the ones whose leaves carry more than a
// body: a page with a transaction pending on it (the pending sets), its
// identity (the directory), the partition ledger (chains that move every block,
// the events root) and the synthetic ledger (the delivery queues).
//
// The entry is the retained receipt's anchor, which is the value the BPT holds
// for the account at that block; a leaf retained from the first of two writes
// in a block would not rebuild it.
func TestRetainedLeafRebuildsTheRetainedEntry(t *testing.T) {
	liteKey := acctesting.GenerateKey("leaf-lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity().JoinPath(ACME)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey("leaf-alice")
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

	secondKey := acctesting.GenerateKey("leaf-alice-2")
	secondHash := sha256.Sum256(secondKey[32:])
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(page).
			Body(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{Entry: KeySpecParams{KeyHash: secondHash[:]}},
				&SetThresholdKeyPageOperation{Threshold: 2},
			}}).
			SignWith(page).Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	pending := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			SignWith(page).Version(2).Timestamp(2).PrivateKey(aliceKey))
	sim.StepUntil(Txn(pending.TxID).IsPending())
	sim.StepN(5)

	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		if isDebugObserver(t, batch, alice) {
			t.Skip("the debug observer does not expose the state components")
		}

		var sawPendingSets, sawDirectory, sawChains bool
		for _, u := range []*url.URL{alice, page, alice.JoinPath("book"), partition.Ledger(), partition.Synthetic()} {
			account := batch.Account(u)
			blocks, err := account.RetainedStateReceiptBlocks().Get()
			require.NoError(t, err)
			if !u.Equal(partition.Synthetic()) {
				// The synthetic ledger changes only when synthetic messages
				// flow, which this network may not have done
				require.NotEmptyf(t, blocks, "nothing retained for %v", u)
			}

			for _, b := range blocks {
				receipt, err := account.RetainedStateReceipt(b).Get()
				require.NoError(t, err)
				leaf, err := account.RetainedLeaf(b).Get()
				require.NoErrorf(t, err, "%v retained a receipt for block %d but no leaf", u, b)

				rebuilt, err := leaf.EntryHash(receipt.Start)
				require.NoError(t, err)
				require.Equalf(t, receipt.Anchor, rebuilt, "%v: the leaf retained for block %d does not rebuild the entry retained for it", u, b)

				for _, p := range append(leaf.Pending, leaf.BookPending...) {
					if len(p.Signatures)+len(p.Votes)+len(p.Payments) > 0 {
						sawPendingSets = true
					}
				}
				sawDirectory = sawDirectory || len(leaf.Directory) > 0
				sawChains = sawChains || len(leaf.Chains) > 0
			}
		}
		require.True(t, sawPendingSets, "no retained leaf carried the sets of a pending transaction, so this proves nothing about them")
		require.True(t, sawDirectory, "no retained leaf carried a directory")
		require.True(t, sawChains, "no retained leaf carried a chain")

		// And the proof carries the leaf
		retained, err := indexing.RetainedBlockRange(partition, batch)
		require.NoError(t, err)
		proof, err := indexing.HistoricalAccountStateProof(partition, batch, batch.Account(partition.Ledger()), retained.Latest)
		require.NoError(t, err)
		require.NotNil(t, proof.Leaf, "the historical proof does not carry the leaf")
		rebuilt, err := proof.Leaf.EntryHash(proof.Receipt.Start)
		require.NoError(t, err)
		require.True(t, passesThrough(proof.Receipt, rebuilt), "the leaf the proof carries is not on the path its receipt proves")
	})
}

// passesThrough reports whether walking the receipt from its start reaches the
// given hash on the way to its anchor.
func passesThrough(r *merkle.Receipt, want []byte) bool {
	h := r.Start
	for _, e := range r.Entries {
		if bytes.Equal(h, want) {
			return true
		}
		var b []byte
		if e.Right {
			b = append(append(b, h...), e.Hash...)
		} else {
			b = append(append(b, e.Hash...), h...)
		}
		v := sha256.Sum256(b)
		h = v[:]
	}
	return bytes.Equal(h, want)
}
