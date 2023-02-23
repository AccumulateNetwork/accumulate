// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestDoubleDelegated(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	charlie := AccountUrl("charlie")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
	)

	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	sim.SetRoute(charlie, "BVN2")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])

	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		p.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
		p.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
	})

	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, protocol.AcmePrecisionPower).
			SignWith(charlie, "book", "1").Version(1).Timestamp(1).PrivateKey(charlieKey).
			Delegator(bob, "book", "1").Delegator(alice, "book", "1")))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}

func TestSingleDelegated(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
	)

	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])

	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		p.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
	})

	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, protocol.AcmePrecisionPower).
			SignWith(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey).
			Delegator(alice, "book", "1")))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}
