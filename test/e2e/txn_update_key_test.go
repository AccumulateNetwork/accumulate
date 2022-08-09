package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestUpdateKey_HasDelegate(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")
	otherKey := acctesting.GenerateKey(alice, "other")

	// Initialize
	var timestamp uint64
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.Keys[0].Delegate = protocol.AccountUrl("foo")
	})
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Update the key
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book", "1")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&UpdateKey{NewKeyHash: hash(otherKey[32:])}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Verify the delegate is unchanged
	page := simulator.GetAccount[*KeyPage](sim, alice.JoinPath("book", "1"))
	require.Len(t, page.Keys, 1)
	require.NotNil(t, page.Keys[0].Delegate)
	require.Equal(t, "foo.acme", page.Keys[0].Delegate.ShortString())
}
