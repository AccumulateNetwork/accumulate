// Copyright 2026 The Accumulate Authors
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

// Anchor recovery under collection proofs (#4087).
//
// Before this, a destination missing anchors waited for the SOURCE to push them:
// every validator independently re-signed and re-submitted each historical
// anchor, consulting the destination once per candidate to see whether it had
// already signed. The query load grew with the size of the gap and was aimed at
// a node that is by definition behind — 1.2M re-submissions and 14 cores in 25
// minutes of the #4067 soak.
//
// Now the destination can pull: one range request, one collection proof covering
// the whole run, no signature quorum to re-gather.
//
// The pull only sees a HOLE — a missing anchor revealed because a later one
// arrived. It cannot see a TAIL, where the most recent anchors never arrive and
// the destination holds no evidence anything is missing; only the source knows
// what it sent unacknowledged. So the source-side push is NOT retired, and this
// test asserts recovery still works with both paths present. Asserting the range
// path specifically requires inducing a hole rather than a dropped tail, which
// DropInitialAnchor does not do — see the follow-up on #4087.

func TestAnchorRecoveryWithCollectionProofsActive(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	bob := build.
		Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	// Drop anchors on their first send, so the only way the transaction
	// completes is recovery.
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice, bob),
		simulator.DropInitialAnchor(),
	)

	// A cross-partition transfer, which cannot settle until the destination has
	// the anchors proving it.
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Sig(st[1].TxID).Completes(),
		Txn(st[0].TxID).Completes())

	// It delivered.
	account, err := bob.Tokens("tokens").Load(sim.DatabaseFor)
	require.NoError(t, err)
	require.Equal(t, 123, int(account.Balance.Int64()))

	// This is the regression guard that matters right now: adding the
	// destination-side pull must not break recovery of a dropped TAIL, which
	// only the source-side push can detect. An earlier version of this change
	// retired the push under Kourou and this assertion failed — nothing
	// recovered at all.
}
