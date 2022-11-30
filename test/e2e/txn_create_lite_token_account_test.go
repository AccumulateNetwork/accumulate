// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
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
		sim := simulator.New(t, 3)
		sim.InitFromGenesis()

		sim.CreateAccount(&LiteIdentity{Url: liteSigner, CreditBalance: 1e9})
		sim.CreateIdentity(alice, aliceKey[32:])
		updateAccount(sim, alicePage, func(p *KeyPage) { p.CreditBalance = 1e9 })

		st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
			acctesting.NewTransaction().
				WithPrincipal(subject).
				WithSigner(c.SignerUrl, 1).
				WithTimestampVar(&timestamp).
				WithBody(&CreateLiteTokenAccount{}).
				Initiate(SignatureTypeED25519, c.SignerKey).
				Build(),
		)...)
		require.False(t, st[0].Failed(), "Expected the transaction to succeed")

		// Verify
		simulator.GetAccount[*LiteTokenAccount](sim, subject)
	})
}
