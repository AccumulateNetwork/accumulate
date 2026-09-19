// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview4348_ADirectoryThatMovesWithNoChain — chain/state_cache.go's
// createOrUpdate puts the new account on ITS OWN main chain and the directory
// entry on its PARENT, and synthetic_deposit_tokens.go adds a directory entry
// to a lite identity that already exists without updating it. So a lite
// identity's directory list grows with no chain of the lite identity moving.
//
// That is the account F5 rests on not existing: every chain equal, the peer's
// state taken whole, and the node's own directory entry gone.
func TestReview4348_ADirectoryThatMovesWithNoChain(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	liteKey := acctesting.GenerateKey("review4348-lite")
	liteId := LiteAuthorityForKey(liteKey[32:], SignatureTypeED25519)
	liteAcme := liteId.JoinPath(ACME)
	liteFoo := liteId.JoinPath(alice.ShortString(), "tokens")

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(liteId, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenIssuer{Url: alice.JoinPath("tokens"), Symbol: "FOO", Precision: 1})
	// The lite identity exists already, with its ACME account.
	MakeAccount(t, sim.DatabaseFor(liteId), &LiteIdentity{Url: liteId})
	MakeAccount(t, sim.DatabaseFor(liteId), &LiteTokenAccount{Url: liteAcme, TokenUrl: AcmeUrl()})
	sim.StepN(5)

	db := sim.DatabaseFor(liteId)
	watch := []*url.URL{liteId}
	part := PartitionUrl("BVN0")
	pullOne := func(from pull.Source, into *database.Database, u *url.URL) (bool, error) {
		bt := into.Begin(true)
		defer bt.Discard()
		p, err := pull.Fetch(context.Background(), from, bt, u,
			pull.Options{Mode: pull.ModeStateOnly, Partition: part}, false)
		if err != nil {
			return false, err
		}
		past := p.Past()
		if err := p.Keep(); err != nil {
			return past, err
		}
		if err := bt.UpdateBPT(); err != nil {
			return past, err
		}
		return past, bt.Commit()
	}

	// The peer: a node that stopped BEFORE the deposit. Built the way a node
	// is built, by pulling the account from the live store as it stands now.
	behind := emptyDb()
	_, err := pullOne(peerServingPart(db, "BVN0"), behind, liteId)
	require.NoError(t, err)

	before := snapOf(t, db, watch)
	t.Logf("lite identity before: chains=%v directory=%v", before[liteId.String()].chains, before[liteId.String()].dir)

	// A deposit of a second token type: the lite identity is not updated, so
	// no chain of it moves, but its directory gains an entry.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			Body(&IssueTokens{Recipient: liteFoo, Amount: *big.NewInt(123)}).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	sim.StepN(3)

	after := snapOf(t, db, watch)
	a, b := before[liteId.String()], after[liteId.String()]
	t.Logf("lite identity after:  chains=%v directory=%v", b.chains, b.dir)
	require.NotEqual(t, a.dir, b.dir, "the directory did not change; the setup is wrong")
	require.True(t, sameChains(a.chains, b.chains),
		"a chain of the lite identity moved: chains %v -> %v", a.chains, b.chains)
	require.NotEqual(t, a.leaf, b.leaf, "the leaf did not move")
	t.Log("**** the lite identity's directory grew and NO chain of it moved")

	// This node: it executed the deposit and came back holding that.
	local := emptyDb()
	_, err = pullOne(peerServingPart(db, "BVN0"), local, liteId)
	require.NoError(t, err)
	require.Equal(t, b.dir, snapOf(t, local, watch)[liteId.String()].dir)

	// One join round against the peer that is behind.
	past, err := pullOne(peerServingPart(behind, "BVN0"), local, liteId)
	require.NoError(t, err)
	got := snapOf(t, local, watch)[liteId.String()]
	t.Logf("after pulling from the stale peer: past=%v directory=%v", past, got.dir)
	require.Equal(t, b.dir, got.dir,
		"the node took the stale peer's directory: the lite identity no longer names one of its own token accounts")
}
