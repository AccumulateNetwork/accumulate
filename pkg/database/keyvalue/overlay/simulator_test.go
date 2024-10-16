// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package overlay_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestOverlay(t *testing.T) {
	// Setup
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	badger := simulator.BadgerDbOpener(t.TempDir(), func(err error) { require.NoError(t, err) })
	simOpts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime).With(alice).WithVersion(ExecutorVersionV2Vandenberg),
	}

	// Execute a transaction with the original database (timestamp = 1)
	sim := NewSim(t, append(simOpts,
		simulator.WithDatabase(badger),
	)...)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Execute a transaction in an overlay (timestamp = 2)
	sim = NewSim(t, append(simOpts,
		simulator.OverlayDatabase(simulator.MemoryDbOpener, badger),
	)...)

	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Verify that the same timestamp fails
	st = sim.BuildAndSubmit(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey))[1]
	require.ErrorIs(t, st.AsError(), errors.BadTimestamp)

	// Executing the transaction with the same timestamp in another overlay succeeds
	sim = NewSim(t, append(simOpts,
		simulator.OverlayDatabase(simulator.MemoryDbOpener, badger),
	)...)

	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st.TxID).Completes())
}
