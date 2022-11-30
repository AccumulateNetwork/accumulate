// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	fuzzutil "gitlab.com/accumulatenetwork/accumulate/test/util/fuzz"
)

func FuzzEnvelopeDecode(f *testing.F) {
	key := acctesting.GenerateKey()
	fuzzutil.AddValue(f, acctesting.NewTransaction().
		WithPrincipal(AccountUrl("foo")).
		WithSigner(AccountUrl("bar"), 1).
		WithTimestamp(1).
		WithBody(&AddCredits{Recipient: AccountUrl("baz"), Amount: *big.NewInt(100000000)}).
		Initiate(SignatureTypeED25519, key).
		Build())

	f.Fuzz(func(t *testing.T, data []byte) {
		t.Parallel()
		_ = new(Transaction).UnmarshalBinary(data)
	})
}

func FuzzAddCredits(f *testing.F) {
	f.Add("foo.acme", "100000000", uint64(InitialAcmeOracleValue))
	f.Add("bar.acme", "200000000", uint64(0))
	f.Add("", "300000000", uint64(InitialAcmeOracleValue))

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)

	f.Fuzz(func(t *testing.T, recipient string, amount string, oracle uint64) {
		t.Parallel()

		sim := simulator.New(t, 3)
		sim.InitFromGenesis()
		sim.CreateLiteTokenAccount(liteKey, AcmeUrl(), 1e9, 1e12)

		_, _ = sim.SubmitAndExecuteBlock(
			acctesting.NewTransaction().
				WithPrincipal(lite).
				WithSigner(lite, 1).
				WithTimestamp(1).
				WithBody(&AddCredits{
					Recipient: fuzzutil.MustParseUrl(t, recipient),
					Amount:    fuzzutil.MustParseBigInt(t, amount),
					Oracle:    oracle,
				}).
				Initiate(SignatureTypeED25519, liteKey).
				Build(),
		)
	})
}

func FuzzUpdateAccountAuth(f *testing.F) {
	fuzzutil.AddValue(f, &UpdateAccountAuth{Operations: []AccountAuthOperation{&AddAccountAuthorityOperation{Authority: AccountUrl("foo")}}})

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	f.Fuzz(func(t *testing.T, data []byte) {
		t.Parallel()

		body := new(UpdateAccountAuth)
		if body.UnmarshalBinary(data) != nil {
			t.Skip()
		}

		sim := simulator.New(t, 3)
		sim.InitFromGenesis()
		sim.CreateIdentity(alice, aliceKey[32:])

		_, _ = sim.SubmitAndExecuteBlock(
			acctesting.NewTransaction().
				WithPrincipal(alice).
				WithSigner(alice.JoinPath("book", "1"), 1).
				WithTimestamp(1).
				WithBody(body).
				Initiate(SignatureTypeED25519, aliceKey).
				Build(),
		)
	})
}
