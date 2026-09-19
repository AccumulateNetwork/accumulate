// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/sha256"
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

// TestReview4348_TheLeafIsNotFourThings — the pull's new comment says the
// account's leaf is "its body, its directory list, its pending list and its
// chains together". observer_prod.go hashes two more things into it: the
// ledger's scheduled-events BPT root, and the synthetic account's delivery
// queues. Neither is pulled. This measures what that costs.
func TestReview4348_TheLeafIsNotFourThings(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	key1 := acctesting.GenerateKey(alice, 1)
	key2 := acctesting.GenerateKey(alice, 2)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
		h := sha256.Sum256(key2[32:])
		p.AddKeySpec(&KeySpec{PublicKeyHash: h[:]})
		p.CreditBalance = 1e9
	})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	sim.StepN(5)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(key1))
	sim.StepUntil(Txn(st.TxID).IsPending())
	sim.StepN(3)

	part := PartitionUrl("BVN0")
	ledger := part.JoinPath(Ledger)
	src := sim.DatabaseFor(alice)

	var eventsRoot [32]byte
	require.NoError(t, src.View(func(b *database.Batch) error {
		h, err := b.Account(ledger).Events().BPT().GetRootHash()
		if err != nil {
			return err
		}
		eventsRoot = h
		return nil
	}))
	t.Logf("the BVN0 ledger's scheduled-events BPT root: %x", eventsRoot)

	// Pull the ledger into an empty node, exactly as the join's long tail does.
	dst := emptyDb()
	require.NoError(t, pullStateOnlyFrom(t, peerServingPart(src, "BVN0"), dst, part, []*url.URL{ledger}))

	var mine, theirs [32]byte
	require.NoError(t, dst.View(func(b *database.Batch) error {
		h, err := b.Account(ledger).Hash()
		mine = h
		return err
	}))
	require.NoError(t, src.View(func(b *database.Batch) error {
		h, err := b.Account(ledger).Hash()
		theirs = h
		return err
	}))
	t.Logf("pulled leaf %x", mine)
	t.Logf("peer's leaf %x", theirs)
	if eventsRoot != ([32]byte{}) {
		require.Equal(t, theirs, mine,
			"the pulled ledger does not hash to the peer's leaf: the events BPT is in the leaf and is never pulled")
	}
}

// TestReview4348_PastKeepsAPendingListItShould — the mirror of the above for
// the past path: an account the node is past on keeps its own pending list.
func TestReview4348_PastPathOnAnAuthority(t *testing.T) {
	alice := url.MustParse("alice")
	key1 := acctesting.GenerateKey(alice, 1)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	sim.StepN(5)

	book := alice.JoinPath("book")
	part := PartitionUrl("BVN0")
	pullBook := func(from pull.Source, into *database.Database) (bool, error) {
		b := into.Begin(true)
		defer b.Discard()
		p, err := pull.Fetch(context.Background(), from, b, book,
			pull.Options{Mode: pull.ModeStateOnly, Partition: part}, false)
		if err != nil {
			return false, err
		}
		past := p.Past()
		if err := p.Keep(); err != nil {
			return past, err
		}
		if err := b.UpdateBPT(); err != nil {
			return past, err
		}
		return past, b.Commit()
	}

	// The peer: the book with one page.
	behind := emptyDb()
	_, err := pullBook(peerServingPart(sim.DatabaseFor(alice), "BVN0"), behind)
	require.NoError(t, err)

	// The node executes on: a second page.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(book).
			CreateKeyPage().WithEntry().Key(acctesting.GenerateKey(alice, 9)[32:], SignatureTypeED25519).FinishEntry().
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(key1))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	sim.StepN(2)

	local := emptyDb()
	_, err = pullBook(peerServingPart(sim.DatabaseFor(alice), "BVN0"), local)
	require.NoError(t, err)

	past, err := pullBook(peerServingPart(behind, "BVN0"), local)
	require.NoError(t, err)
	t.Logf("pulling a book from a peer one page behind: past=%v", past)
	require.True(t, past, "the node was not past the peer on the book")
}
