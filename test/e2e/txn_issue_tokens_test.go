// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestIssueTokens_Good(t *testing.T) {
	var timestamp uint64

	// Initialize
	values := new(core.GlobalValues)
	values.ExecutorVersion = ExecutorVersionLatest
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(values)

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenIssuer{Url: alice.JoinPath("tokens"), Symbol: "FOO", Precision: 1})
	liteKey := acctesting.GenerateKey("lite")
	lite := LiteAuthorityForKey(liteKey[32:], SignatureTypeED25519).JoinPath(alice.ShortString(), "tokens")

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&IssueTokens{
				Recipient: lite,
				Amount:    *big.NewInt(123),
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	liteAcct := simulator.GetAccount[*LiteTokenAccount](sim, lite)
	require.Equal(t, 123, int(liteAcct.Balance.Int64()))
}

func TestIssueTokens_Bad(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenIssuer{Url: alice.JoinPath("tokens"), Symbol: "FOO", Precision: 1})
	liteKey := acctesting.GenerateKey("lite")
	lite := LiteAuthorityForKey(liteKey[32:], SignatureTypeED25519).JoinPath(ACME)

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&IssueTokens{
				Recipient: lite,
				Amount:    *big.NewInt(123),
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// All issued tokens should be returned
	issuer := simulator.GetAccount[*TokenIssuer](sim, alice.JoinPath("tokens"))
	require.Zero(t, issuer.Issued.Int64())
}

func TestIssueTokens_Multi(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })
	sim.CreateAccount(&TokenIssuer{Url: alice.JoinPath("tokens"), Symbol: "FOO", Precision: 1})

	lite1Key, lite2Key := acctesting.GenerateKey("lite", 1), acctesting.GenerateKey("lite", 2)
	lite1 := LiteAuthorityForKey(lite1Key[32:], SignatureTypeED25519).JoinPath(alice.ShortString(), "tokens")
	lite2 := LiteAuthorityForKey(lite2Key[32:], SignatureTypeED25519).JoinPath(alice.ShortString(), "tokens")

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&IssueTokens{
				To: []*TokenRecipient{
					{Url: lite1, Amount: *big.NewInt(123)},
					{Url: lite2, Amount: *big.NewInt(456)},
				},
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
	)...)

	// Verify
	require.Equal(t, 123, int(simulator.GetAccount[*LiteTokenAccount](sim, lite1).Balance.Int64()))
	require.Equal(t, 456, int(simulator.GetAccount[*LiteTokenAccount](sim, lite2).Balance.Int64()))
}
