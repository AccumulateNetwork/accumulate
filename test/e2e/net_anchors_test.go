// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestDropInitialAnchor is a simple test that simulates adverse network
// conditions causing anchors to be dropped when they're initially sent.
func TestDropInitialAnchor(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),

		// Drop anchors when they are initially sent, instead relying on the
		// Conductor's anchor healing
		simulator.DropInitialAnchor,
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

	// Verify
	account := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens"))
	require.Equal(t, 123, int(account.Balance.Int64()))
}

// TestIdenticalAnchors verifies that the anchors the DN sends are identical
// regardless of where they're going.
func TestIdenticalAnchors(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Do something
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Wait for anchoring to settle
	sim.StepN(20)

	// Get the latest DN anchor
	var anchors []*protocol.Transaction
	for _, p := range sim.Partitions() {
		var anchor *protocol.Transaction
		View(t, sim.Database(p.ID), func(batch *database.Batch) {
			chain := batch.Account(PartitionUrl(p.ID).JoinPath(AnchorPool)).MainChain()
			head, err := chain.Head().Get()
			require.NoError(t, err)

			for i := head.Count - 1; i >= 0; i-- {
				hash, err := chain.Entry(i)
				require.NoError(t, err)
				var txn *messaging.TransactionMessage
				err = batch.Message2(hash).Main().GetAs(&txn)
				require.NoError(t, err)

				if txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
					anchor = txn.Transaction
					break
				}
			}
		})
		require.NotNil(t, anchor, "Cannot find a DN anchor for %s", p.ID)
		anchors = append(anchors, anchor)
	}

	a, err := json.MarshalIndent(anchors[0], "", "  ")
	require.NoError(t, err)
	for _, b := range anchors[1:] {
		b, err := json.MarshalIndent(b, "", "  ")
		require.NoError(t, err)
		assert.Equal(t, string(a), string(b))
	}
	if !t.Failed() {
		t.Log("All anchors match: ", string(a))
	}
}
