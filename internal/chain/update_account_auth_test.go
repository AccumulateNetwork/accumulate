package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
