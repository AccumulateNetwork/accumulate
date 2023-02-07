// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestUpdateAccountAuth_Duplicate(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice)
	sim.CreateAccount(&protocol.TokenAccount{Url: alice.JoinPath("tokens")})

	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("tokens")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{
			&protocol.AddAccountAuthorityOperation{
				Authority: alice.JoinPath("book"),
			},
		}}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	x := sim.PartitionFor(alice)
	st, txn := chain.LoadStateManagerForTest(t, x.Database, env)
	defer st.Discard()

	_, err := chain.UpdateAccountAuth{}.Execute(st, txn)
	require.EqualError(t, err, "duplicate authority "+alice.JoinPath("book").String())
}

func TestUpdateAccountAuth_Page(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	alice := protocol.AccountUrl("alice")
	bob := protocol.AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateIdentity(bob, bobKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })
	updateAccount(sim, bob.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&protocol.TokenAccount{
		Url: alice.JoinPath("tokens"),
		AccountAuth: protocol.AccountAuth{
			Authorities: []protocol.AuthorityEntry{
				{Url: alice.JoinPath("book")},
				{Url: bob.JoinPath("book")},
			},
		},
	})

	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("tokens")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{
			&protocol.AddAccountAuthorityOperation{
				Authority: alice.JoinPath("book", "1"),
			},
		}}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		WithSigner(bob.JoinPath("book", "1"), 1).
		Sign(protocol.SignatureTypeED25519, bobKey).
		Build()

	_, err := sim.SubmitAndExecuteBlock(env)
	require.EqualError(t, err, "invalid authority acc://alice.acme/book/1: a key page is not a valid authority")
}
