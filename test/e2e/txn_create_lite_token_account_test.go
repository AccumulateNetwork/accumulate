// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCreateLiteTokenAccount(t *testing.T) {
	type Case struct {
		SignerUrl *url.URL // URL of the page/lite id
		SignerKey []byte   // Key of the page/lite id
	}

	liteKey := acctesting.GenerateKey("Lite")
	liteSigner := acctesting.AcmeLiteAddressStdPriv(liteKey).RootIdentity()
	alice := AccountUrl("alice")
	alicePage := alice.JoinPath("book", "1")
	aliceKey := acctesting.GenerateKey(alice)

	subject := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Subject"))

	cases := map[string]Case{
		"With Page": {alicePage, aliceKey},
		"With Lite": {liteSigner, liteKey},
	}

	Run(t, cases, func(t *testing.T, c Case) {
		var timestamp uint64

		// Initialize
		sim := NewSim(t,
			simulator.MemoryDatabase,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.Genesis(GenesisTime),
		)

		MakeAccount(t, sim.DatabaseFor(liteSigner), &LiteIdentity{Url: liteSigner, CreditBalance: 1e9})
		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		UpdateAccount(t, sim.DatabaseFor(alice), alicePage, func(p *KeyPage) { p.CreditBalance = 1e9 })

		st := sim.SubmitSuccessfully(MustBuild(t,
			build.Transaction().For(subject).
				CreateLiteTokenAccount().
				SignWith(c.SignerUrl).Version(1).Timestamp(&timestamp).PrivateKey(c.SignerKey)))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())

		// Verify
		GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(subject), subject)
	})
}
