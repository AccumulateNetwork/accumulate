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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview4348_TheDeliveryQueuesAreInTheLeafAndNeverPulled — the synthetic
// account's local and cascade delivery queues are hashed into its BPT leaf
// (observer_prod.go, #4146/#4155) and the pull does not carry them. A node
// that pulls the account cannot reproduce the leaf while a queue is non-empty.
func TestReview4348_TheDeliveryQueuesAreInTheLeafAndNeverPulled(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	part := PartitionUrl("BVN0")
	synth := part.JoinPath(Synthetic)
	db := sim.S.Database("BVN0")

	queued := func() int {
		b := db.Begin(false)
		defer b.Discard()
		l, err := b.Account(synth).LocalDeliveryQueue().Get()
		require.NoError(t, err)
		c, err := b.Account(synth).CascadeDeliveryQueue().Get()
		require.NoError(t, err)
		return len(l) + len(c)
	}

	var ts uint64
	found := false
	for i := 0; i < 40 && !found; i++ {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		for j := 0; j < 4 && !found; j++ {
			sim.Step()
			if queued() > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Skip("no block in this run left a delivery queue non-empty")
	}
	t.Logf("the synthetic account holds %d queued deliveries", queued())

	dst := emptyDb()
	require.NoError(t, pullStateOnlyFrom(t, peerServingPart(db, "BVN0"), dst, part, []*url.URL{synth}))

	var mine, theirs [32]byte
	require.NoError(t, dst.View(func(b *database.Batch) error {
		h, err := b.Account(synth).Hash()
		mine = h
		return err
	}))
	b := db.Begin(false)
	h, err := b.Account(synth).Hash()
	require.NoError(t, err)
	b.Discard()
	theirs = h
	t.Logf("pulled leaf %x", mine)
	t.Logf("peer's leaf %x", theirs)
	require.Equal(t, theirs, mine,
		"the pulled synthetic account does not hash to the peer's leaf: the delivery queues are in the leaf and are never pulled")
}
