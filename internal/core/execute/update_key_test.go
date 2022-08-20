package execute_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

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

func TestUpdateKey_Duplicate(t *testing.T) {
	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	otherKey := acctesting.GenerateKey(alice, "other")
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()
	sim.CreateIdentity(alice, aliceKey[32:], otherKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *protocol.KeyPage) { p.CreditBalance = 1e9 })

	// Sign with other key, update to alice key
	env := acctesting.NewTransaction().
		WithPrincipal(alice.JoinPath("book", "1")).
		WithSigner(alice.JoinPath("book", "1"), 1).
		WithTimestamp(1).
		WithBody(&protocol.UpdateKey{
			NewKeyHash: doHash(aliceKey[32:]),
		}).
		Initiate(protocol.SignatureTypeED25519, otherKey).
		Build()

	st, err := sim.SubmitAndExecuteBlock(env)
	require.NoError(t, err)
	require.EqualError(t, st[0].Error, "cannot have duplicate entries on key page")
}
