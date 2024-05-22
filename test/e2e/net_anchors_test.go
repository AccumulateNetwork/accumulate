// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestDropInitialAnchor is a simple test that simulates adverse network
// conditions causing anchors to be dropped when they're initially sent.
func TestDropInitialAnchor(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	bob := build.
		Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice, bob),

		// Drop anchors when they are initially sent, instead relying on the
		// Conductor's anchor healing
		simulator.DropInitialAnchor(),
	)

	// Execute
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Sig(st[1].TxID).Completes(),
		Txn(st[0].TxID).Completes())

	// Verify
	account, err := bob.Tokens("tokens").Load(sim.DatabaseFor)
	require.NoError(t, err)
	require.Equal(t, 123, int(account.Balance.Int64()))
}
