// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestWalk(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	var keys []string
	View(t, sim.DatabaseFor(alice), func(batch *coredb.Batch) {
		require.NoError(t, batch.ForEachAccount(func(account *coredb.Account, hash [32]byte) error {
			return account.Walk(database.WalkOptions{
				Values:        true,
				IgnoreIndices: true,
			}, func(r database.Record) (skip bool, err error) {
				// fmt.Println(r.Key())
				keys = append(keys, r.Key().String())
				return false, nil
			})
		}))
	})

	require.Contains(t, keys, "Account.acc://alice/tokens.Url")
	require.Contains(t, keys, "Account.acc://alice/tokens.Main")
	require.Contains(t, keys, "Account.acc://alice/tokens.MainChain.Head")
	require.Contains(t, keys, "Account.acc://alice/tokens.MainChain.Index.Head")
	require.Contains(t, keys, "Account.acc://alice/tokens.SignatureChain.Head")
	require.Contains(t, keys, "Account.acc://alice/tokens.SignatureChain.Index.Head")
}
