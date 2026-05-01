// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestBPTRepair verifies the mechanism we plan to use to repair the
// 21 stale BPT entries on Cyclops mainnet: a transaction whose
// principal is the corrupted account (or which routes credits to it)
// marks that account dirty, and UpdateBPT at block-end refreshes the
// leaf to the current account.Hash(). The test simulates the
// corruption by manually overwriting the BPT entry, confirms the
// corruption took, runs a repair-style transaction, and verifies the
// leaf is now consistent.
//
// Three account classes are exercised, covering the diverse repair
// mechanisms in /tmp/repair-cyclops-bpt.sh:
//   - ADI                 → updateAccountAuth (principal marked dirty)
//   - lite identity       → addCredits to recipient (recipient dirty)
//   - lite data account   → writeData (principal dirty)
//
// Token issuer / lite token / key book follow the same dirty-mark
// pathways and are not separately exercised — the executor's
// UpdateBPT walk treats all dirty accounts uniformly.
func TestBPTRepair(t *testing.T) {
	const stalePrefix = byte(0xC0)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// ADI: alice (signs everything; funds the credits/data writes)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Lite identity: provisioned via faucet helper.
	liteKey := acctesting.GenerateKey(t.Name(), "Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], AcmeUrl())
	liteRoot := lite.RootIdentity()

	// LDA: instantiated by writing to it.
	ldaEntry := &FactomDataEntry{ExtIds: [][]byte{[]byte("bpt-repair-test")}}
	ldaEntry.AccountId = *(*[32]byte)(ComputeLiteDataAccountId(ldaEntry.Wrap()))
	ldaUrl, err := LiteDataAddress(ldaEntry.AccountId[:])
	require.NoError(t, err)
	{
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(ldaUrl).
				Body(&WriteData{Entry: ldaEntry.Wrap()}).
				SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
	}

	type target struct {
		name string
		url  *url.URL
		// repair runs a transaction whose effect is to mark `url` dirty.
		repair func(t *testing.T, ts *uint64)
	}

	var ts uint64 = 100
	targets := []target{
		{
			name: "ADI",
			url:  alice,
			repair: func(t *testing.T, ts *uint64) {
				// Create a fresh sub-account under alice. Principal of
				// CreateDataAccount is the parent (alice). On commit
				// alice's Directory list grows by one entry → alice is
				// marked dirty → BPT leaf refreshes.
				st := sim.BuildAndSubmitTxnSuccessfully(
					build.Transaction().For(alice).
						Body(&CreateDataAccount{Url: alice.JoinPath("repair-data")}).
						SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(ts).PrivateKey(aliceKey))
				sim.StepUntilN(50, Txn(st.TxID).Completes())
			},
		},
		{
			name: "LiteIdentity",
			url:  liteRoot,
			repair: func(t *testing.T, ts *uint64) {
				// Spend 1 ACME to buy a comfortable batch of credits.
				st := sim.BuildAndSubmitTxnSuccessfully(
					build.Transaction().For(alice.JoinPath("tokens")).
						Body(&AddCredits{
							Recipient: liteRoot,
							Amount:    *big.NewInt(1 * AcmePrecision),
							Oracle:    InitialAcmeOracleValue,
						}).
						SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(ts).PrivateKey(aliceKey))
				sim.StepUntilN(50, Txn(st.TxID).Completes())
			},
		},
		{
			name: "LiteDataAccount",
			url:  ldaUrl,
			repair: func(t *testing.T, ts *uint64) {
				entry := &DoubleHashDataEntry{Data: [][]byte{[]byte("repair")}}
				st := sim.BuildAndSubmitTxnSuccessfully(
					build.Transaction().For(ldaUrl).
						Body(&WriteData{Entry: entry}).
						SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(ts).PrivateKey(aliceKey))
				sim.StepUntilN(50, Txn(st.TxID).Completes())
			},
		},
	}

	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			db := sim.DatabaseFor(tt.url)

			// 1. Capture the current (correct) leaf hash.
			var preHash [32]byte
			View(t, db, func(batch *database.Batch) {
				h, err := batch.Account(tt.url).Hash()
				require.NoError(t, err)
				preHash = h
			})
			t.Logf("%s pre-corruption account.Hash() = %x", tt.name, preHash[:8])

			// 2. Corrupt the BPT entry with a deliberately-wrong leaf.
			fake := [32]byte{}
			for i := range fake {
				fake[i] = stalePrefix
			}
			Update(t, db, func(batch *database.Batch) {
				key := record.NewKey("Account", tt.url)
				require.NoError(t, batch.BPT().Insert(key, fake[:]))
			})

			// 3. Verify the corruption: the BPT entry no longer matches
			//    account.Hash(). This is the state Cyclops is actually
			//    in for 21 specific accounts on mainnet.
			View(t, db, func(batch *database.Batch) {
				key := record.NewKey("Account", tt.url)
				stored, err := batch.BPT().Get(key)
				require.NoError(t, err)
				require.Equal(t, fake[:], stored,
					"BPT entry should be the planted fake")

				h, err := batch.Account(tt.url).Hash()
				require.NoError(t, err)
				require.NotEqual(t, fake, h,
					"account.Hash() must not equal the planted fake (otherwise the test is testing nothing)")
			})

			// 4. Run a repair transaction: any txn that marks the
			//    principal dirty.
			ts++
			tt.repair(t, &ts)

			// 5. The BPT leaf should be back in sync.
			View(t, db, func(batch *database.Batch) {
				key := record.NewKey("Account", tt.url)
				stored, err := batch.BPT().Get(key)
				require.NoError(t, err)
				require.NotEqual(t, fake[:], stored,
					"BPT entry should have been refreshed off the planted fake")

				h, err := batch.Account(tt.url).Hash()
				require.NoError(t, err)
				require.Equal(t, h[:], stored,
					"BPT leaf should match account.Hash() after the repair txn")
			})
		})
	}
}
