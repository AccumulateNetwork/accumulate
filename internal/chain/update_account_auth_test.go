package chain_test

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var delivered = (*protocol.TransactionStatus).Delivered

func updateAccount[T protocol.Account](sim *simulator.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account protocol.Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		if err != nil {
			sim.Log(err)
			sim.FailNow()
		}

		fn(typed)
	})
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

	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)

	account := simulator.GetAccount[*protocol.TokenAccount](sim, alice.JoinPath("tokens"))
	t.Log(account)
}
