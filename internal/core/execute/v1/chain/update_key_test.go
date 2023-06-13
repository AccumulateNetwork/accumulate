// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func updateAccount[T protocol.Account](sim *simulator.Simulator, accountUrl *url.URL, fn func(account T)) {
	sim.UpdateAccount(accountUrl, func(account protocol.Account) {
		var typed T
		err := encoding.SetPtr(account, &typed)
		require.NoError(sim.TB, err)
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
	env :=
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("book", "1")).
			Body(&protocol.UpdateKey{
				NewKeyHash: doHash(aliceKey[32:]),
			}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(otherKey).Type(protocol.SignatureTypeED25519))

	st := sim.H.SubmitTxnSuccessfully(env)
	sim.H.StepUntil(
		Txn(st.TxID).Fails())

	msg := sim.H.QueryTransaction(st.TxID, nil)
	require.EqualError(t, msg.AsError(), "cannot have duplicate entries on key page")
}
